package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ipm/pkg/cache"
	"ipm/pkg/log"
	"ipm/pkg/registry"
	"ipm/pkg/solver"
	"ipm/pkg/types"

	"github.com/Masterminds/semver/v3"
)

type Installer struct {
	cache     *cache.Cache
	installed map[string]string
	solver    *solver.Solver
}

func NewInstaller(reg registry.Registry) *Installer {
	c, _ := cache.NewCache()
	return &Installer{
		cache:     c,
		installed: make(map[string]string),
		solver:    solver.NewSolver(reg),
	}
}

func (i *Installer) Install(reg registry.Registry, pkgSpec string, jsonOutput bool, pubKeyFile string) error {
	// Prüfe, ob pkgSpec eine lokale Datei ist
	if _, err := os.Stat(pkgSpec); err == nil {
		log.Debug("Detected local package file", map[string]interface{}{
			"file": pkgSpec,
		})
		f, err := os.Open(pkgSpec)
		if err != nil {
			return fmt.Errorf("failed to open local package file: %v", err)
		}
		defer f.Close()

		// Tarball lesen
		tarballData, err := io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("failed to read local tarball: %v", err)
		}

		// Signatur prüfen
		if pubKeyFile != "" {
			if err := verifyTarball(tarballData, pubKeyFile); err != nil {
				return err
			}
		}

		// Metadaten extrahieren
		pkg, err := extractPackageMetadata(tarballData)
		if err != nil {
			return fmt.Errorf("failed to extract package metadata: %v", err)
		}

		// Installation fortsetzen
		return i.installLocalPackage(reg, pkg, tarballData, jsonOutput, pubKeyFile)
	}

	// Registry-Installation (bestehende Logik)
	name, version := parsePackageSpec(pkgSpec)
	if version == "" {
		version = "latest"
	}

	log.Info("Starting installation", map[string]interface{}{
		"package": name,
		"version": version,
	})

	pkg := types.Package{Name: name, Version: version}
	if i.cache.Exists(pkg) {
		var err error
		pkg, err = i.cache.LoadMetadata(pkg)
		if err != nil {
			log.Error("Failed to load cached metadata", err, map[string]interface{}{
				"package": name,
				"version": version,
			})
			return err
		}
		return i.installCachedDep(reg, pkg, jsonOutput, pubKeyFile)
	}

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
		pkg.Version = version
	}

	if existingVersion, ok := i.installed[name]; ok {
		if existingVersion != pkg.Version {
			log.Info("Package already installed with different version, skipping", map[string]interface{}{
				"package":   name,
				"existing":  existingVersion,
				"requested": pkg.Version,
			})
			return nil
		}
		return nil
	}

	fmt.Printf("Installing %s@%s...\n", name, pkg.Version)
	tarballReader, fetchedPkg, err := reg.FetchPackageTarball(name, pkg.Version)
	if err != nil {
		log.Error("Failed to fetch package tarball", err, map[string]interface{}{
			"package": name,
			"version": pkg.Version,
		})
		return err
	}
	defer tarballReader.Close()

	if pubKeyFile != "" {
		tarballData, err := io.ReadAll(tarballReader)
		if err != nil {
			return fmt.Errorf("failed to read tarball: %v", err)
		}
		if err := verifyTarball(tarballData, pubKeyFile); err != nil {
			return err
		}
		tarballReader = io.NopCloser(bytes.NewReader(tarballData))
	}

	pkg = fetchedPkg
	cachedPath, err := i.cache.Store(pkg, tarballReader)
	if err != nil {
		log.Error("Failed to store package in cache", err, map[string]interface{}{
			"package": name,
			"version": pkg.Version,
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
			"version": pkg.Version,
		})
		return err
	}

	i.installed[name] = pkg.Version
	log.Info("Package installed", map[string]interface{}{
		"package": pkg.Name,
		"version": pkg.Version,
		"path":    cachedPath,
	})
	fmt.Printf("Installed %s@%s to %s\n", pkg.Name, pkg.Version, cachedPath)

	for depName, depVersion := range pkg.Deps {
		if err := i.installDependency(reg, depName, depVersion, jsonOutput, pubKeyFile); err != nil {
			return err
		}
	}

	return nil
}

func (i *Installer) installLocalPackage(reg registry.Registry, pkg types.Package, tarballData []byte, jsonOutput bool, pubKeyFile string) error {
	if existingVersion, ok := i.installed[pkg.Name]; ok {
		if existingVersion != pkg.Version {
			log.Info("Package already installed with different version, skipping", map[string]interface{}{
				"package":   pkg.Name,
				"existing":  existingVersion,
				"requested": pkg.Version,
			})
			return nil
		}
		return nil
	}

	cachedPath, err := i.cache.Store(pkg, io.NopCloser(bytes.NewReader(tarballData)))
	if err != nil {
		log.Error("Failed to store package in cache", err, map[string]interface{}{
			"package": pkg.Name,
			"version": pkg.Version,
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
			"package": pkg.Name,
			"version": pkg.Version,
		})
		return err
	}

	i.installed[pkg.Name] = pkg.Version
	log.Info("Package installed", map[string]interface{}{
		"package": pkg.Name,
		"version": pkg.Version,
		"path":    cachedPath,
	})
	fmt.Printf("Installed %s@%s to %s\n", pkg.Name, pkg.Version, cachedPath)

	for depName, depVersion := range pkg.Deps {
		if err := i.installDependency(reg, depName, depVersion, jsonOutput, pubKeyFile); err != nil {
			return err
		}
	}

	return nil
}

func verifyTarball(tarballData []byte, pubKeyFile string) error {
	pubKeyData, err := os.ReadFile(pubKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read public key: %v", err)
	}
	block, _ := pem.Decode(pubKeyData)
	if block == nil {
		return fmt.Errorf("invalid public key format")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %v", err)
	}

	publicKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not an RSA key")
	}

	gzr, err := gzip.NewReader(bytes.NewReader(tarballData))
	if err != nil {
		return fmt.Errorf("failed to read gzip: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var signature []byte
	var unsignedData []byte

	tempFile, err := os.CreateTemp("", "unsigned-*.tgz")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	gw := gzip.NewWriter(tempFile)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tarball: %v", err)
		}
		if hdr.Name == "signature.sig" {
			signature, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("failed to read signature: %v", err)
			}
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}
		_, err = io.Copy(tw, tr)
		if err != nil {
			return fmt.Errorf("failed to copy file: %v", err)
		}
	}

	tw.Close()
	gw.Close()
	tempFile.Close()

	if signature == nil {
		log.Warn("Package is not signed", map[string]interface{}{
			"file": "downloaded tarball",
		})
		return nil
	}

	unsignedData, err = os.ReadFile(tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to read unsigned tarball: %v", err)
	}

	hash := sha256.Sum256(unsignedData)
	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature)
	if err != nil {
		return fmt.Errorf("package signature verification failed: %v", err)
	}

	log.Info("Package signature verified", map[string]interface{}{
		"file": "downloaded tarball",
	})
	return nil
}

func extractPackageMetadata(tarballData []byte) (types.Package, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(tarballData))
	if err != nil {
		return types.Package{}, fmt.Errorf("failed to read gzip: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return types.Package{}, fmt.Errorf("failed to read tarball: %v", err)
		}
		if strings.HasSuffix(hdr.Name, "package.json") {
			data, err := io.ReadAll(tr)
			if err != nil {
				return types.Package{}, fmt.Errorf("failed to read package.json: %v", err)
			}
			var pkg struct {
				Name         string            `json:"name"`
				Version      string            `json:"version"`
				Dependencies map[string]string `json:"dependencies"`
			}
			if err := json.Unmarshal(data, &pkg); err != nil {
				return types.Package{}, fmt.Errorf("failed to parse package.json: %v", err)
			}
			return types.Package{
				Name:    pkg.Name,
				Version: pkg.Version,
				Deps:    pkg.Dependencies,
			}, nil
		}
	}
	return types.Package{}, fmt.Errorf("package.json not found in tarball")
}

func (i *Installer) installDependency(reg registry.Registry, depName, depVersion string, jsonOutput bool, pubKeyFile string) error {
	if installedVersion, ok := i.installed[depName]; ok {
		if satisfiesVersion(installedVersion, depVersion) {
			log.Debug("Using already installed dependency version", map[string]interface{}{
				"package": depName,
				"version": installedVersion,
				"range":   depVersion,
			})
			return nil
		}
	}

	cachedDep := types.Package{Name: depName}
	if i.cache.HasCachedVersion(depName) {
		versions, err := i.cache.GetCachedVersions(depName)
		if err == nil && len(versions) > 0 {
			for _, v := range versions {
				cachedDep.Version = v
				if i.cache.Exists(cachedDep) {
					cachedDep, err = i.cache.LoadMetadata(cachedDep)
					if err == nil && satisfiesVersion(cachedDep.Version, depVersion) {
						log.Debug("Using cached dependency version directly", map[string]interface{}{
							"package": depName,
							"version": cachedDep.Version,
							"range":   depVersion,
						})
						return i.installCachedDep(reg, cachedDep, jsonOutput, pubKeyFile)
					}
				}
			}
		}
	}

	resolvedVersion, err := reg.ResolveVersion(depName, depVersion)
	if err != nil {
		log.Error("Failed to resolve dependency version", err, map[string]interface{}{
			"package": depName,
			"version": depVersion,
		})
		return err
	}
	cachedDep.Version = resolvedVersion
	if i.cache.Exists(cachedDep) {
		cachedDep, err = i.cache.LoadMetadata(cachedDep)
		if err == nil {
			log.Debug("Using resolved cached dependency", map[string]interface{}{
				"package": depName,
				"version": cachedDep.Version,
				"range":   depVersion,
			})
			return i.installCachedDep(reg, cachedDep, jsonOutput, pubKeyFile)
		}
	}

	return i.Install(reg, fmt.Sprintf("%s@%s", depName, resolvedVersion), jsonOutput, pubKeyFile)
}

func (i *Installer) installCachedDep(reg registry.Registry, pkg types.Package, jsonOutput bool, pubKeyFile string) error {
	if existingVersion, ok := i.installed[pkg.Name]; ok {
		if existingVersion != pkg.Version {
			log.Info("Cached dependency already installed with different version, skipping", map[string]interface{}{
				"package":   pkg.Name,
				"existing":  existingVersion,
				"requested": pkg.Version,
			})
			return nil
		}
		log.Debug("Cached dependency already installed", map[string]interface{}{
			"package": pkg.Name,
			"version": pkg.Version,
		})
		return nil
	}

	cachedPath := filepath.Join(i.cache.CacheDir, fmt.Sprintf("%s-%s", pkg.Name, pkg.Version))
	pkgDir := filepath.Join("node_modules")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		log.Error("Failed to create node_modules directory", err, map[string]interface{}{
			"dir": pkgDir,
		})
		return err
	}

	if err := i.cache.Link(pkg, pkgDir); err != nil {
		log.Error("Failed to link cached dependency", err, map[string]interface{}{
			"package": pkg.Name,
			"version": pkg.Version,
		})
		return err
	}

	i.installed[pkg.Name] = pkg.Version
	log.Info("Cached dependency installed", map[string]interface{}{
		"package": pkg.Name,
		"version": pkg.Version,
		"path":    cachedPath,
	})

	for depName, depVersion := range pkg.Deps {
		if err := i.installDependency(reg, depName, depVersion, jsonOutput, pubKeyFile); err != nil {
			return err
		}
	}

	return nil
}

func satisfiesVersion(version, rangeSpec string) bool {
	if rangeSpec == "latest" {
		return false
	}

	ver, err := semver.NewVersion(version)
	if err != nil {
		log.Debug("Invalid version format, treating as exact match", map[string]interface{}{
			"version": version,
		})
		return version == rangeSpec
	}

	constraint, err := semver.NewConstraint(rangeSpec)
	if err != nil {
		log.Debug("Invalid range format, treating as exact match", map[string]interface{}{
			"range": rangeSpec,
		})
		return version == rangeSpec
	}

	return constraint.Check(ver)
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