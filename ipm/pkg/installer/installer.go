package installer

import (
	"encoding/json"
	"fmt"
	"ipm/pkg/cache"
	"ipm/pkg/log"
	"ipm/pkg/registry"
	"ipm/pkg/solver"
	"os"
	"path/filepath"
)

type Installer struct {
	cache     *cache.Cache
	installed map[string]string
	solver    *solver.Solver
}

func NewInstaller(reg registry.Registry) *Installer {
	c, _ := cache.NewCache() // Fehlerbehandlung später
	return &Installer{
		cache:     c,
		installed: make(map[string]string),
		solver:    solver.NewSolver(reg),
	}
}

func (i *Installer) Install(reg registry.Registry, pkgSpec string, jsonOutput bool) error {
	name, version := parsePackageSpec(pkgSpec)
	if version == "" {
		version = "latest"
	}

	log.Info("Starting installation", map[string]interface{}{
		"package": name,
		"version": version,
	})

	if err := i.solver.AddPackage(name, version); err != nil {
		log.Error("Failed to analyze dependencies", err, map[string]interface{}{
			"package": name,
			"version": version,
		})
		return err
	}
	if i.solver.HasConflicts() {
		i.reportConflicts(jsonOutput)
		os.Exit(1)
	}

	if version != "latest" && (version[0] == '~' || version[0] == '^' || version[0] == '>') {
		resolvedVersion, err := reg.ResolveVersion(name, version)
		if err != nil {
			log.Error("Failed to resolve version", err, map[string]interface{}{
				"package": name,
				"version": version,
			})
			return err
		}
		version = resolvedVersion
		log.Debug("Resolved version", map[string]interface{}{
			"package": name,
			"from":    version,
			"to":      resolvedVersion,
		})
	}

	if existingVersion, ok := i.installed[name]; ok {
		if existingVersion != version {
			log.Info("Package already installed with different version, skipping", map[string]interface{}{
				"package":   name,
				"existing":  existingVersion,
				"requested": version,
			})
			return nil
		}
		return nil
	}

	fmt.Printf("Installing %s@%s...\n", name, version)

	tarball, pkg, err := reg.FetchPackageTarball(name, version)
	if err != nil {
		log.Error("Failed to fetch package tarball", err, map[string]interface{}{
			"package": name,
			"version": version,
		})
		return err
	}

	cachedPath, err := i.cache.Store(pkg, tarball)
	if err != nil {
		log.Error("Failed to store package in cache", err, map[string]interface{}{
			"package": name,
			"version": version,
		})
		return err
	}

	pkgDir := filepath.Join("node_modules")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		log.Error("Failed to create node_modules directory", err, map[string]interface{}{
			"dir": pkgDir,
		})
		return err
	}

	if err := i.cache.Link(pkg, pkgDir); err != nil {
		log.Error("Failed to link package", err, map[string]interface{}{
			"package": name,
			"version": version,
		})
		return err
	}

	i.installed[name] = version
	log.Info("Package installed", map[string]interface{}{
		"package": pkg.Name,
		"version": pkg.Version,
		"path":    cachedPath,
	})
	fmt.Printf("Installed %s@%s to %s\n", pkg.Name, pkg.Version, cachedPath)

	for depName, depVersion := range pkg.Deps {
		if err := i.Install(reg, fmt.Sprintf("%s@%s", depName, depVersion), jsonOutput); err != nil {
			return err
		}
	}

	return nil
}

func (i *Installer) reportConflicts(jsonOutput bool) {
	if jsonOutput {
		conflicts := i.solver.GetConflicts()
		type conflictOutput struct {
			Package    string   `json:"package"`
			Versions   []string `json:"versions"`
			Dependents []string `json:"dependents"`
		}
		output := struct {
			Message   string           `json:"message"`
			Conflicts []conflictOutput `json:"conflicts"`
			Error     string           `json:"error"`
		}{
			Message:   "Installation failed due to dependency conflicts",
			Conflicts: make([]conflictOutput, len(conflicts)),
			Error:     "unresolvable dependency conflicts detected",
		}
		for i, c := range conflicts {
			output.Conflicts[i] = conflictOutput{
				Package:    c.Package,
				Versions:   c.Versions,
				Dependents: c.Dependents,
			}
		}
		jsonData, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(jsonData))
	} else {
		fmt.Println("Installation failed due to dependency conflicts:")
		for _, conflict := range i.solver.GetConflicts() {
			fmt.Printf("- Conflict at '%s':\n", conflict.Package)
			fmt.Printf("  Versions requested: %v\n", conflict.Versions)
			fmt.Printf("  Dependents: %v\n", conflict.Dependents)
		}
		fmt.Println("Error: unresolvable dependency conflicts detected")
	}
	log.Error("Unresolvable dependency conflicts detected", nil)
}

func parsePackageSpec(spec string) (name, version string) {
	parts := []rune(spec)
	for i, r := range parts {
		if r == '@' {
			return string(parts[:i]), string(parts[i+1:])
		}
	}
	return spec, ""
}