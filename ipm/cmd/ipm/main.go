package main

import (
	"fmt"
	"os"

	"ipm/pkg/installer"
	"ipm/pkg/log"
	"ipm/pkg/registry"

	"github.com/spf13/cobra"
)

var (
	registryURL string
	logLevel    string
	logFile     string
)

var rootCmd = &cobra.Command{Use: "ipm"}

var installCmd = &cobra.Command{
	Use:   "install [package]",
	Short: "Install a package",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pubKeyFile, _ := cmd.Flags().GetString("pubkey") // Lokales Flag
		if err := log.Init(logLevel, logFile); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		log.Debug("Starting installation process", map[string]interface{}{
			"package": args[0],
			"pubkey":  pubKeyFile,
		})
		reg := registry.NewNPMRegistry(registryURL, "")
		inst := installer.NewInstaller(reg)
		if err := inst.Install(reg, args[0], false, pubKeyFile); err != nil {
			log.Error("Installation failed", err)
			os.Exit(1)
		}
		log.Info("Installation completed", map[string]interface{}{
			"package": args[0],
		})
	},
}

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Initialize a new package",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := log.Init(logLevel, logFile); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		if err := initPackage(args[0]); err != nil {
			log.Error("Failed to initialize package", err)
			os.Exit(1)
		}
		fmt.Printf("Initialized package %s\n", args[0])
	},
}

var packCmd = &cobra.Command{
	Use:   "pack [directory]",
	Short: "Pack a package into a .tgz file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := log.Init(logLevel, logFile); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		tarball, err := packPackage(args[0])
		if err != nil {
			log.Error("Failed to pack package", err)
			os.Exit(1)
		}
		fmt.Printf("Packed package to %s\n", tarball)
	},
}

var signCmd = &cobra.Command{
	Use:   "sign [file]",
	Short: "Sign a package file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		keyFile, _ := cmd.Flags().GetString("key")
		if err := log.Init(logLevel, logFile); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		log.Debug("Starting signing process", map[string]interface{}{
			"file": args[0],
			"key":  keyFile,
		})
		if err := signPackage(args[0], keyFile); err != nil {
			log.Error("Failed to sign package", err)
			os.Exit(1)
		}
		log.Info("Package signed successfully", map[string]interface{}{
			"file": args[0],
		})
		fmt.Printf("Signed package %s\n", args[0])
	},
}

var verifyCmd = &cobra.Command{
	Use:   "verify [file]",
	Short: "Verify a package file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pubKeyFile, _ := cmd.Flags().GetString("pubkey")
		if err := log.Init(logLevel, logFile); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		log.Debug("Starting verification process", map[string]interface{}{
			"file":   args[0],
			"pubkey": pubKeyFile,
		})
		if err := verifyPackage(args[0], pubKeyFile); err != nil {
			fmt.Printf("Package verification failed: %v\n", err)
			log.Error("Failed to verify package", err)
			os.Exit(1)
		}
		log.Info("Package verified successfully", map[string]interface{}{
			"file": args[0],
		})
		fmt.Printf("Verified package %s\n", args[0])
	},
}

func main() {
	rootCmd.PersistentFlags().StringVar(&registryURL, "registry", "https://registry.npmjs.org", "Registry URL")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level (debug, info, error)")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Log file path")

	// Kommando-spezifische Flags
	installCmd.Flags().String("pubkey", "", "Public key file for signature verification")
	signCmd.Flags().String("key", "", "Private key file for signing")
	verifyCmd.Flags().String("pubkey", "", "Public key file for verification")

	rootCmd.AddCommand(installCmd, initCmd, packCmd, signCmd, verifyCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}