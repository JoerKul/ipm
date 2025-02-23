package cli

import (
	"fmt"
	"ipm/pkg/installer"
	"ipm/pkg/log"
	"ipm/pkg/registry"

	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
    var logLevel string
    var logFile string
    var jsonOutput bool

    rootCmd := &cobra.Command{
        Use:   "ipm",
        Short: "Industrial Package Manager",
        Long:  "A secure, extensible package manager for industrial applications.",
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            return log.Init(logLevel, logFile)
        },
    }

    reg := registry.NewNPMRegistry()
    inst := installer.NewInstaller()

    installCmd := &cobra.Command{
        Use:   "install [package[@version]]",
        Short: "Install a package",
        Args:  cobra.MinimumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            return inst.Install(reg, args[0], jsonOutput)
        },
    }

    rootCmd.AddCommand(installCmd)
    rootCmd.AddCommand(&cobra.Command{
        Use:   "version",
        Short: "Print the version",
        Run: func(cmd *cobra.Command, args []string) {
            if jsonOutput {
                fmt.Println(`{"version": "0.1.0"}`)
            } else {
                fmt.Println("IPM v0.1.0")
            }
        },
    })

    rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Set log level (debug, info, error) to enable logging")
    rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Write logs to specified file")
    rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

    return rootCmd
}