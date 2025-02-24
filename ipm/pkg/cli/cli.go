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
	var registryURL string
	var registryToken string

	rootCmd := &cobra.Command{
		Use:   "ipm",
		Short: "Industrial Package Manager",
		Long:  "A secure, extensible package manager for industrial applications.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Zuerst Logger initialisieren
			if err := log.Init(logLevel, logFile); err != nil {
				return err
			}
			// Danach Debug-Log schreiben
			log.Debug("Initializing with registry configuration", map[string]interface{}{
				"registryURL":  registryURL,
				"registryToken": registryToken,
			})
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&registryURL, "registry", "https://registry.npmjs.org", "Custom registry URL (e.g., https://npm.pkg.github.com)")
	rootCmd.PersistentFlags().StringVar(&registryToken, "token", "", "Authentication token for the registry")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Set log level (debug, info, error) to enable logging")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Write logs to specified file")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	// Registry und Installer nach Flag-Definition initialisieren
	reg := registry.NewNPMRegistry(registryURL, registryToken)
	inst := installer.NewInstaller(reg)

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

	return rootCmd
}