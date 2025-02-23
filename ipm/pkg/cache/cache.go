package cache

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
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

func (c *Cache) Store(pkg types.Package, tarball io.ReadCloser) (string, error) {
    defer tarball.Close()

    pkgPath := filepath.Join(c.CacheDir, fmt.Sprintf("%s-%s", pkg.Name, pkg.Version))
    if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
        // Entpacke Tarball
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

            // Entferne "package/" Präfix aus npm-Tarballs
            targetPath := filepath.Join(pkgPath, strings.TrimPrefix(header.Name, "package/"))

            switch header.Typeflag {
            case tar.TypeDir:
                if err := os.MkdirAll(targetPath, 0755); err != nil {
                    return "", fmt.Errorf("failed to create dir %s: %v", targetPath, err)
                }
            case tar.TypeReg:
                // Erstelle das Verzeichnis für die Datei
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
    }
    return pkgPath, nil
}

func (c *Cache) Link(pkg types.Package, targetDir string) error {
    linkPath := filepath.Join(targetDir, pkg.Name)
    cachedPath := filepath.Join(c.CacheDir, fmt.Sprintf("%s-%s", pkg.Name, pkg.Version))

    // Prüfe, ob der Link schon existiert
    if _, err := os.Lstat(linkPath); err == nil {
        if err := os.Remove(linkPath); err != nil {
            return fmt.Errorf("failed to remove existing link %s: %v", linkPath, err)
        }
    }

    return os.Symlink(cachedPath, linkPath)
}