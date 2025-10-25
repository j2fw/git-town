package cmd

import (
	"fmt"

	"github.com/git-town/git-town/v22/internal/cli/flags"
	"github.com/git-town/git-town/v22/internal/cli/print"
	"github.com/git-town/git-town/v22/internal/cmd/cmdhelpers"
	"github.com/git-town/git-town/v22/internal/config/cliconfig"
	"github.com/git-town/git-town/v22/internal/config/configdomain"
	"github.com/git-town/git-town/v22/internal/execute"
	"github.com/git-town/git-town/v22/internal/forge"
	"github.com/git-town/git-town/v22/internal/git/gitdomain"
	"github.com/git-town/git-town/v22/pkg/prelude"
	"github.com/spf13/cobra"
)

const (
	prsDesc = "Display proposals for the branch hierarchy"
	prsHelp = `
TODO`
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
	// data, flow, err := determineBranchData(repo)
	// if err != nil {
	// 	return err
	// }
	data, flow, err := determinePRsData(repo)
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

	for _, branch := range data.branches {
		fmt.Println(branch)
	}

	// fmt.Print(branchLayout(entries, data))
	return nil
}

type prsData struct {
	branches gitdomain.LocalBranchNames
}

func determinePRsData(repo execute.OpenRepoResult) (data prsData, flow configdomain.ProgramFlow, err error) {
	repoStatus, err := repo.Git.RepoStatus(repo.Backend)
	if err != nil {
		return data, configdomain.ProgramFlowExit, err
	}

	config := repo.UnvalidatedConfig.NormalConfig
	connector, err := forge.NewConnector(forge.NewConnectorArgs{
		Backend: repo.Backend,
		// BitbucketAppPassword: config.BitbucketAppPassword,
		// BitbucketUsername:    config.BitbucketUsername,
		ForgeType: config.ForgeType,
		// ForgejoToken:         config.ForgejoToken,
		Frontend:            repo.Frontend,
		GitHubConnectorType: config.GitHubConnectorType,
		GitHubToken:         config.GitHubToken,
		// GitLabConnectorType:  config.GitLabConnectorType,
		// GitLabToken:          config.GitLabToken,
		// GiteaToken:           config.GiteaToken,
		Log:       print.Logger{},
		RemoteURL: config.DevURL(repo.Backend),
	})
	if err != nil {
		return data, configdomain.ProgramFlowExit, err
	}

	branchesSnapshot, _, _, flow, err := execute.LoadRepoSnapshot(execute.LoadRepoSnapshotArgs{
		Backend:               repo.Backend,
		CommandsCounter:       repo.CommandsCounter,
		ConfigSnapshot:        repo.ConfigSnapshot,
		Connector:             connector,
		Fetch:                 false,
		FinalMessages:         repo.FinalMessages,
		Frontend:              repo.Frontend,
		Git:                   repo.Git,
		HandleUnfinishedState: false,
		// Inputs:                inputs,
		Repo:                  repo,
		RepoStatus:            repoStatus,
		RootDir:               repo.RootDir,
		UnvalidatedConfig:     repo.UnvalidatedConfig,
		ValidateNoOpenChanges: false,
	})
	if err != nil {
		return data, configdomain.ProgramFlowExit, err
	}

	switch flow {
	case configdomain.ProgramFlowContinue:
	case configdomain.ProgramFlowExit, configdomain.ProgramFlowRestart:
		return data, flow, nil
	}

	branchesAndTypes := repo.UnvalidatedConfig.UnvalidatedBranchesAndTypes(branchesSnapshot.Branches.LocalBranches().NamesLocalBranches())
	localBranches := branchesSnapshot.Branches.LocalBranches().NamesLocalBranches()
	perennialBranches := branchesAndTypes.BranchesOfTypes(configdomain.BranchTypePerennialBranch, configdomain.BranchTypeMainBranch)

	branchesToWalk := gitdomain.LocalBranchNames{}
	branchesToWalk = localBranches.Remove(perennialBranches...)

	return prsData{
		branches: branchesToWalk,
	}, configdomain.ProgramFlowContinue, nil
}
