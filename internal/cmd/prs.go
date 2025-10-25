package cmd

import (
	"fmt"
	"regexp"

	"github.com/git-town/git-town/v22/internal/cli/dialog"
	"github.com/git-town/git-town/v22/internal/cli/flags"
	"github.com/git-town/git-town/v22/internal/cmd/cmdhelpers"
	"github.com/git-town/git-town/v22/internal/config/cliconfig"
	"github.com/git-town/git-town/v22/internal/config/configdomain"
	"github.com/git-town/git-town/v22/internal/execute"
	"github.com/git-town/git-town/v22/internal/git/gitdomain"
	"github.com/git-town/git-town/v22/pkg/prelude"
	"github.com/spf13/cobra"
)

const (
	prsDesc = "Display proposals for the branch hierarchy"
	prsHelp = `
Git Town's equivalent of the "git branch" command.`
)

func prsCmd() *cobra.Command {
	addVerboseFlag, readVerboseFlag := flags.Verbose()
	cmd := cobra.Command{
		Use:     "proposals",
		Aliases: []string{"prs"},
		Args:    cobra.NoArgs,
		Short:   prsDesc,
		Long:    cmdhelpers.Long(prsDesc, prsHelp),
		RunE: func(cmd *cobra.Command, args []string) error {
			verbose, errVerbose := readVerboseFlag(cmd)
			if err := errVerbose; err != nil {
				return err
			}
			cliConfig := cliconfig.New(cliconfig.NewArgs{
				AutoResolve:  prelude.Option[configdomain.AutoResolve]{},
				AutoSync:     prelude.Option[configdomain.AutoSync]{},
				Detached:     prelude.Option[configdomain.Detached]{},
				DryRun:       prelude.Option[configdomain.DryRun]{},
				Order:        prelude.Option[configdomain.Order]{},
				PushBranches: prelude.Option[configdomain.PushBranches]{},
				Stash:        prelude.Option[configdomain.Stash]{},
				Verbose:      verbose,
			})
			return executePRs(cliConfig)
		},
	}

	addVerboseFlag(&cmd)
	return &cmd
}

func executePRs(cliConfig configdomain.PartialConfig) error {
Start:
	repo, err := execute.OpenRepo(execute.OpenRepoArgs{
		CliConfig:        cliConfig,
		PrintBranchNames: true,
		PrintCommands:    true,
		ValidateGitRepo:  true,
		ValidateIsOnline: false,
	})
	if err != nil {
		return err
	}
	data, flow, err := determineBranchData(repo)
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
	entries := dialog.NewSwitchBranchEntries(dialog.NewSwitchBranchEntriesArgs{
		BranchInfos:       data.branchInfos,
		BranchTypes:       []configdomain.BranchType{},
		BranchesAndTypes:  data.branchesAndTypes,
		ExcludeBranches:   gitdomain.LocalBranchNames{},
		Lineage:           repo.UnvalidatedConfig.NormalConfig.Lineage,
		MainBranch:        repo.UnvalidatedConfig.UnvalidatedConfig.MainBranch,
		Order:             repo.UnvalidatedConfig.NormalConfig.Order,
		Regexes:           []*regexp.Regexp{},
		ShowAllBranches:   false,
		UnknownBranchType: repo.UnvalidatedConfig.NormalConfig.UnknownBranchType,
	})
	fmt.Print(branchLayout(entries, data))
	return nil
}
