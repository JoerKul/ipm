package cache

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"ipm/pkg/log"
	"ipm/pkg/types"
	"os"
	"path/filepath"
	"strings"
)

type Cache struct {
	CacheDir string
}

func NewCache() (*Cache, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	cacheDir := filepath.Join(home, ".ipm", "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}
	return &Cache{CacheDir: cacheDir}, nil
}

func (c *Cache) HasCachedVersion(name string) bool {
	dir := filepath.Join(c.CacheDir, name+"-*")
	matches, _ := filepath.Glob(dir)
	return len(matches) > 0
}

func (c *Cache) GetCachedVersions(name string) ([]string, error) {
	dir := filepath.Join(c.CacheDir, name+"-*")
	matches, err := filepath.Glob(dir)
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(matches))
	for _, match := range matches {
		base := filepath.Base(match)
		version := strings.TrimPrefix(base, name+"-")
		versions = append(versions, version)
	}
	return versions, nil
}

func (c *Cache) Store(pkg types.Package, tarball io.ReadCloser) (string, error) {
	defer tarball.Close()

	pkgPath := filepath.Join(c.CacheDir, fmt.Sprintf("%s-%s", pkg.Name, pkg.Version))
	if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
		log.Debug("Cache miss, storing package", map[string]interface{}{
			"package": pkg.Name,
			"version": pkg.Version,
			"path":    pkgPath,
		})
		if err := os.MkdirAll(pkgPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create cache dir %s: %v", pkgPath, err)
		}

		gz, err := gzip.NewReader(tarball)
		if err != nil {
			return "", fmt.Errorf("failed to create gzip reader: %v", err)
		}
		defer gz.Close()

		tr := tar.NewReader(gz)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("failed to read tar: %v", err)
			}

			targetPath := filepath.Join(pkgPath, strings.TrimPrefix(header.Name, "package/"))

			switch header.Typeflag {
			case tar.TypeDir:
				if err := os.MkdirAll(targetPath, 0755); err != nil {
					return "", fmt.Errorf("failed to create dir %s: %v", targetPath, err)
				}
			case tar.TypeReg:
				dir := filepath.Dir(targetPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return "", fmt.Errorf("failed to create parent dir %s: %v", dir, err)
				}

				file, err := os.Create(targetPath)
				if err != nil {
					return "", fmt.Errorf("failed to create file %s: %v", targetPath, err)
				}
				if _, err := io.Copy(file, tr); err != nil {
					file.Close()
					return "", fmt.Errorf("failed to write file %s: %v", targetPath, err)
				}
				file.Close()
				if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
					return "", fmt.Errorf("failed to set permissions for %s: %v", targetPath, err)
				}
			}
		}

		// Metadaten speichern
		metaPath := filepath.Join(c.CacheDir, fmt.Sprintf("%s-%s.json", pkg.Name, pkg.Version))
		metaData, err := json.Marshal(pkg)
		if err != nil {
			return "", fmt.Errorf("failed to marshal package metadata: %v", err)
		}
		if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
			return "", fmt.Errorf("failed to write package metadata: %v", err)
		}
	} else {
		log.Debug("Cache hit, package already stored", map[string]interface{}{
			"package": pkg.Name,
			"version": pkg.Version,
			"path":    pkgPath,
		})
	}
	return pkgPath, nil
}

func (c *Cache) Link(pkg types.Package, targetDir string) error {
	linkPath := filepath.Join(targetDir, pkg.Name)
	cachedPath := filepath.Join(c.CacheDir, fmt.Sprintf("%s-%s", pkg.Name, pkg.Version))

	if info, err := os.Lstat(linkPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(linkPath); err == nil && target == cachedPath {
				log.Debug("Symlink already exists and is valid", map[string]interface{}{
					"package": pkg.Name,
					"version": pkg.Version,
					"link":    linkPath,
					"target":  cachedPath,
				})
				return nil
			}
			log.Debug("Removing outdated symlink", map[string]interface{}{
				"package": pkg.Name,
				"version": pkg.Version,
				"link":    linkPath,
			})
			if err := os.Remove(linkPath); err != nil {
				return fmt.Errorf("failed to remove existing link %s: %v", linkPath, err)
			}
		}
	}

	log.Debug("Creating new symlink", map[string]interface{}{
		"package": pkg.Name,
		"version": pkg.Version,
		"link":    linkPath,
		"target":  cachedPath,
	})
	return os.Symlink(cachedPath, linkPath)
}

func (c *Cache) Exists(pkg types.Package) bool {
	pkgPath := filepath.Join(c.CacheDir, fmt.Sprintf("%s-%s", pkg.Name, pkg.Version))
	_, err := os.Stat(pkgPath)
	return !os.IsNotExist(err)
}

func (c *Cache) LoadMetadata(pkg types.Package) (types.Package, error) {
	metaPath := filepath.Join(c.CacheDir, fmt.Sprintf("%s-%s.json", pkg.Name, pkg.Version))
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return types.Package{}, fmt.Errorf("failed to read metadata: %v", err)
	}
	var cachedPkg types.Package
	if err := json.Unmarshal(data, &cachedPkg); err != nil {
		return types.Package{}, fmt.Errorf("failed to unmarshal metadata: %v", err)
	}
	log.Debug("Loaded metadata from cache", map[string]interface{}{
		"package": cachedPkg.Name,
		"version": cachedPkg.Version,
		"range":   pkg.Version,
	})
	return cachedPkg, nil
}