package cmd

import (
	"cmp"
	"fmt"

	"github.com/git-town/git-town/v22/internal/cli/flags"
	"github.com/git-town/git-town/v22/internal/cmd/cmdhelpers"
	"github.com/git-town/git-town/v22/internal/cmd/sync"
	"github.com/git-town/git-town/v22/internal/config/cliconfig"
	"github.com/git-town/git-town/v22/internal/config/configdomain"
	"github.com/git-town/git-town/v22/internal/execute"
	"github.com/git-town/git-town/v22/internal/git/gitdomain"
	"github.com/git-town/git-town/v22/internal/state/runstate"
	"github.com/git-town/git-town/v22/internal/vm/interpreter/fullinterpreter"
	"github.com/git-town/git-town/v22/internal/vm/opcodes"
	"github.com/git-town/git-town/v22/internal/vm/optimizer"
	"github.com/git-town/git-town/v22/internal/vm/program"
	"github.com/git-town/git-town/v22/pkg/prelude"
	. "github.com/git-town/git-town/v22/pkg/prelude"
	"github.com/git-town/git-town/v22/pkg/set"
	"github.com/spf13/cobra"
)

const (
	prsDesc = "Display proposals for the branch hierarchy"
	prsHelp = `
TODO`
)

func prsCmd() *cobra.Command {
	// TODO: fix string here
	addStackFlag, readStackFlag := flags.Stack("propose the entire stack")
	addDryRunFlag, readDryRunFlag := flags.DryRun()
	addVerboseFlag, readVerboseFlag := flags.Verbose()
	cmd := cobra.Command{
		Use:     "proposals",
		Aliases: []string{"prs"},
		Args:    cobra.NoArgs,
		Short:   prsDesc,
		Long:    cmdhelpers.Long(prsDesc, prsHelp),
		RunE: func(cmd *cobra.Command, args []string) error {
			verbose, errVerbose := readVerboseFlag(cmd)
			dryRun, errDryRun := readDryRunFlag(cmd)
			stack, errStack := readStackFlag(cmd)
			if err := cmp.Or(errVerbose, errDryRun, errStack); err != nil {
				return err
			}
			cliConfig := cliconfig.New(cliconfig.NewArgs{
				AutoResolve:  prelude.Option[configdomain.AutoResolve]{},
				AutoSync:     prelude.Option[configdomain.AutoSync]{},
				Detached:     prelude.Option[configdomain.Detached]{},
				DryRun:       dryRun,
				Order:        prelude.Option[configdomain.Order]{},
				PushBranches: prelude.Option[configdomain.PushBranches]{},
				Stash:        prelude.Option[configdomain.Stash]{},
				Verbose:      verbose,
			})
			return executePRs(prsArgs{
				cliConfig: cliConfig,
				stack:     stack,
			})
		},
	}
	addStackFlag(&cmd)
	addDryRunFlag(&cmd)
	addVerboseFlag(&cmd)
	return &cmd
}

type prsArgs struct {
	cliConfig configdomain.PartialConfig
	stack     configdomain.FullStack
}

func executePRs(args prsArgs) error {
Start:
	repo, err := execute.OpenRepo(execute.OpenRepoArgs{
		CliConfig:        args.cliConfig,
		PrintBranchNames: true,
		PrintCommands:    true,
		ValidateGitRepo:  true,
		ValidateIsOnline: false,
	})
	if err != nil {
		return err
	}
	pa := prsArgs{
		// body:      None[gitdomain.ProposalBody](),
		// bodyFile:  None[gitdomain.ProposalBodyFile](),
		cliConfig: args.cliConfig,
		stack:     args.stack,
	}
	data, flow, err := determinePRsData(repo, pa)
	if err != nil {
		return err
	}
	switch flow {
	case configdomain.ProgramFlowContinue:
	case configdomain.ProgramFlowExit:
		return nil
	case configdomain.ProgramFlowRestart:
		goto Start
	}
	runProgram := prsProgram(repo, data)
	runState := runstate.RunState{
		BeginBranchesSnapshot: data.branchesSnapshot,
		BeginConfigSnapshot:   repo.ConfigSnapshot,
		BeginStashSize:        data.stashSize,
		BranchInfosLastRun:    data.branchInfosLastRun,
		Command:               proposeCmd,
		DryRun:                data.config.NormalConfig.DryRun,
		EndBranchesSnapshot:   None[gitdomain.BranchesSnapshot](),
		EndConfigSnapshot:     None[configdomain.EndConfigSnapshot](),
		EndStashSize:          None[gitdomain.StashSize](),
		RunProgram:            runProgram,
		TouchedBranches:       runProgram.TouchedBranches(),
		UndoAPIProgram:        program.Program{},
	}
	return fullinterpreter.Execute(fullinterpreter.ExecuteArgs{
		Backend:                 repo.Backend,
		CommandsCounter:         repo.CommandsCounter,
		Config:                  data.config,
		Connector:               data.connector,
		FinalMessages:           repo.FinalMessages,
		Frontend:                repo.Frontend,
		Git:                     repo.Git,
		HasOpenChanges:          data.hasOpenChanges,
		InitialBranch:           data.initialBranch,
		InitialBranchesSnapshot: data.branchesSnapshot,
		InitialConfigSnapshot:   repo.ConfigSnapshot,
		InitialStashSize:        data.stashSize,
		Inputs:                  data.inputs,
		PendingCommand:          None[string](),
		RootDir:                 repo.RootDir,
		RunState:                runState,
	})
}

type prsData struct {
	proposeData
}

func determinePRsData(repo execute.OpenRepoResult, args prsArgs) (data prsData, flow configdomain.ProgramFlow, err error) {
	proposeArgs := proposeArgs{
		// body:      None[gitdomain.ProposalBody](),
		// bodyFile:  None[gitdomain.ProposalBodyFile](),
		cliConfig: args.cliConfig,
		stack:     false,
		title:     prelude.Option[gitdomain.ProposalTitle]{},
	}
	proposeData, flow, err := determineProposeData(repo, proposeArgs)

	return prsData{
		proposeData: proposeData,
	}, configdomain.ProgramFlowContinue, nil
}

func prsProgram(repo execute.OpenRepoResult, data prsData) program.Program {
	prog := NewMutable(&program.Program{})
	data.config.CleanupLineage(data.branchInfos, data.nonExistingBranches, repo.FinalMessages, repo.Backend, data.config.NormalConfig.Order)
	branchesToDelete := set.New[gitdomain.LocalBranchName]()
	sync.BranchesProgram(data.branchesToSync, sync.BranchProgramArgs{
		BranchInfos:         data.branchInfos,
		BranchInfosPrevious: data.branchInfosLastRun,
		BranchesToDelete:    NewMutable(&branchesToDelete),
		Config:              data.config,
		InitialBranch:       data.initialBranch,
		PrefetchBranchInfos: data.preFetchBranchInfos,
		Remotes:             data.remotes,
		Program:             prog,
		Prune:               false,
		PushBranches:        true,
	})

	fmt.Println(data.branchesToPropose)

	for _, branchToPropose := range data.branchesToPropose {
		switch branchToPropose.branchType {
		case configdomain.BranchTypePrototypeBranch, configdomain.BranchTypeParkedBranch, configdomain.BranchTypeContributionBranch, configdomain.BranchTypeMainBranch, configdomain.BranchTypeObservedBranch, configdomain.BranchTypePerennialBranch:
			continue
		}
		prog.Value.Add(&opcodes.PushCurrentBranchIfLocal{
			CurrentBranch: branchToPropose.name,
		})

		if existingProposalURL, hasExistingProposal := branchToPropose.existingProposalURL.Get(); hasExistingProposal {
			fmt.Println("HAS", branchToPropose)
			prog.Value.Add(
				&opcodes.BrowserOpen{
					URL: existingProposalURL,
				},
			)
		} else {
			fmt.Println("ELSE", branchToPropose)
			prog.Value.Add(&opcodes.ProposalCreate{
				Branch:        branchToPropose.name,
				MainBranch:    data.config.ValidatedConfigData.MainBranch,
				ProposalBody:  data.proposalBody,
				ProposalTitle: data.proposalTitle,
			})
		}
	}
	return optimizer.Optimize(prog.Immutable())
}
