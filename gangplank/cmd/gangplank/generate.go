package main

import (
	"fmt"
	"os"
	"time"

	jobspec "github.com/coreos/gangplank/spec"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	// generateFileName is a file handle where the generate JobSpec
	// will be written to with generateCommand
	generateFileName string

	// generateSingleCommands is a list of command that will be run in the stage
	generateCommands []string

	// generateSingleStage indicates that all commands/artfiacts should be in the same stage
	generateSingleStage bool

	// generateSingleRequires insdicates all the artifacts that can be required
	generateSingleRequires []string
)

var (
	// cmdGenerate creates a jobspec and dumps it.
	cmdGenerate = &cobra.Command{
		Use:   "generate",
		Short: "generate jobspec from CLI args",
		Run:   generateCLICommand,
	}

	// generateSinglePod creates a single pod specification
	cmdGenerateSingle = &cobra.Command{
		Use:   "generateSinglePod",
		Short: "generate a single pod job spec",
		Run: func(c *cobra.Command, args []string) {
			generateSingleStage = true
			generateCLICommand(c, args)
		},
	}
)

func init() {
	cmdRoot.PersistentFlags().StringSliceVarP(&automaticBuildStages, "build-artifact", "A", []string{},
		fmt.Sprintf("build artifact for any of: %v", jobspec.GetArtifactShortHandNames()))

	// Add the jobspec flags to the CLI
	spec.AddCliFlags(cmdGenerate.Flags())
	spec.AddCliFlags(cmdGenerateSingle.Flags())

	// Define cmdGenerate flags
	cmdRoot.AddCommand(cmdGenerate)
	cmdGenerate.Flags().StringVar(&generateFileName, "yaml-out", "", "write YAML to file")
	jobspec.AddKolaTestFlags(&cosaKolaTests, cmdGenerate.Flags())

	// Define cmdGenerateSingle flags
	cmdRoot.AddCommand(cmdGenerateSingle)
	cmdGenerateSingle.Flags().StringVar(&generateFileName, "yaml-out", "", "write YAML to file")
	cmdGenerateSingle.Flags().StringSliceVar(&generateCommands, "cmd", []string{}, "commands to run in stage")
	cmdGenerateSingle.Flags().StringSliceVar(&generateSingleRequires, "req", []string{}, "artifacts to require")
	jobspec.AddKolaTestFlags(&cosaKolaTests, cmdGenerateSingle.Flags())
}

// setCliSpec reads or generates a jobspec based on CLI arguments.
func setCliSpec() {
	defer func() {
		// Always add repos
		if spec.Recipe.Repos == nil {
			spec.AddRepos()
		}
		if minioSshRemoteHost != "" {
			log.WithField("ssh host", minioSshRemoteHost).Info("Minio will be forwarded to remote host")
			spec.Job.MinioSSHForward = minioSshRemoteHost
			spec.Job.MinioSSHUser = minioSshRemoteUser
		}
		if minioCfgFile != "" {
			spec.Job.MinioCfgFile = minioCfgFile
		}
	}()

	if specFile != "" {
		js, err := jobspec.JobSpecFromFile(specFile)
		if err != nil {
			log.WithError(err).Fatal("failed to read jobspec")
		}
		spec = js

		log.WithFields(log.Fields{
			"jobspec":          specFile,
			"ingored cli args": "-A|--artifact|--singleReq|--singleCmd",
		}).Info("Using jobspec from file, some cli arguments will be ignored")
		return
	}

	log.Info("Generating jobspec from CLI arguments")
	if len(generateCommands) != 0 || len(generateSingleRequires) != 0 {
		log.Info("--cmd and --req forces single stage mode, only one stage will be run")
		generateSingleStage = true
	}

	log.Info("Generating stages")
	if err := spec.GenerateStages(automaticBuildStages, cosaKolaTests, generateSingleStage); err != nil {
		log.WithError(err).Fatal("failed to generate the jobpsec")
	}

	if spec.Stages == nil {
		spec.Stages = []jobspec.Stage{
			{
				ID:             "CLI Commands",
				ExecutionOrder: 1,
			},
		}
	}

	spec.Stages[0].AddCommands(generateCommands)
	spec.Stages[0].AddRequires(generateSingleRequires)
}

// generateCLICommand is the full spec generator command
func generateCLICommand(*cobra.Command, []string) {
	var out *os.File = os.Stdout
	if generateFileName != "" {
		f, err := os.OpenFile(generateFileName, os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			log.WithError(err).Fatalf("unable to open %s for writing", generateFileName)
		}
		defer f.Close()
		out = f
	}
	setCliSpec()
	defer out.Sync() //nolint

	now := time.Now().Format(time.RFC3339)
	if _, err := out.Write([]byte("# Generated by Gangplank CLI\n# " + now + "\n")); err != nil {
		log.WithError(err).Fatalf("Failed to write header to file")
	}
	if err := spec.WriteYAML(out); err != nil {
		log.WithError(err).Fatal("Faield to write Gangplank YAML")
	}
}
