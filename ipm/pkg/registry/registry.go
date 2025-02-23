package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"ipm/pkg/log"
	"ipm/pkg/types"
	"net/http"

	"github.com/Masterminds/semver/v3"
)

type Registry interface {
    FetchPackageTarball(name, version string) (io.ReadCloser, types.Package, error)
    ResolveVersion(name, versionRange string) (string, error)
}

type NPMRegistry struct {
    BaseURL string
}

func NewNPMRegistry() *NPMRegistry {
    return &NPMRegistry{BaseURL: "https://registry.npmjs.org"}
}

func (r *NPMRegistry) FetchPackageTarball(name, version string) (io.ReadCloser, types.Package, error) {
    metadataURL := fmt.Sprintf("%s/%s/%s", r.BaseURL, name, version)
    log.Debug("Sending request to npm registry", map[string]interface{}{
        "url": metadataURL,
    })

    resp, err := http.Get(metadataURL)
    if err != nil {
        log.Error("Failed to fetch metadata", err, map[string]interface{}{
            "package": name,
            "version": version,
        })
        return nil, types.Package{}, fmt.Errorf("failed to fetch metadata for %s@%s: %v", name, version, err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        log.Error("Metadata request failed", nil, map[string]interface{}{
            "status": resp.Status,
            "url":    metadataURL,
        })
        return nil, types.Package{}, fmt.Errorf("metadata request failed with status: %s", resp.Status)
    }

    var pkgData struct {
        Name         string            `json:"name"`
        Version      string            `json:"version"`
        Dist         struct {
            Tarball string `json:"tarball"`
        } `json:"dist"`
        Dependencies map[string]string `json:"dependencies"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&pkgData); err != nil {
        log.Error("Failed to parse metadata", err, map[string]interface{}{
            "package": name,
            "version": version,
        })
        return nil, types.Package{}, fmt.Errorf("failed to parse metadata: %v", err)
    }

    log.Debug("Package metadata fetched", map[string]interface{}{
        "package": pkgData.Name,
        "version": pkgData.Version,
    })

    tarballResp, err := http.Get(pkgData.Dist.Tarball)
    if err != nil {
        log.Error("Failed to fetch tarball", err, map[string]interface{}{
            "url": pkgData.Dist.Tarball,
        })
        return nil, types.Package{}, fmt.Errorf("failed to fetch tarball for %s@%s: %v", name, version, err)
    }
    if tarballResp.StatusCode != http.StatusOK {
        tarballResp.Body.Close()
        log.Error("Tarball request failed", nil, map[string]interface{}{
            "status": tarballResp.Status,
            "url":    pkgData.Dist.Tarball,
        })
        return nil, types.Package{}, fmt.Errorf("tarball request failed with status: %s", tarballResp.Status)
    }

    pkg := types.Package{
        Name:    pkgData.Name,
        Version: pkgData.Version,
        Deps:    pkgData.Dependencies,
    }
    return tarballResp.Body, pkg, nil
}

func (r *NPMRegistry) ResolveVersion(name, versionRange string) (string, error) {
    metadataURL := fmt.Sprintf("%s/%s", r.BaseURL, name)
    log.Debug("Sending request to npm registry for version resolution", map[string]interface{}{
        "url": metadataURL,
    })

    resp, err := http.Get(metadataURL)
    if err != nil {
        log.Error("Failed to fetch metadata for version resolution", err, map[string]interface{}{
            "package": name,
        })
        return "", fmt.Errorf("failed to fetch metadata for %s: %v", name, err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        log.Error("Metadata request failed", nil, map[string]interface{}{
            "status": resp.Status,
            "url":    metadataURL,
        })
        return "", fmt.Errorf("metadata request failed with status: %s", resp.Status)
    }

    var pkgData struct {
        Versions map[string]interface{} `json:"versions"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&pkgData); err != nil {
        log.Error("Failed to parse metadata", err, map[string]interface{}{
            "package": name,
        })
        return "", fmt.Errorf("failed to parse metadata: %v", err)
    }

    constraint, err := semver.NewConstraint(versionRange)
    if err != nil {
        log.Error("Invalid version range", err, map[string]interface{}{
            "range": versionRange,
        })
        return "", fmt.Errorf("invalid version range %s: %v", versionRange, err)
    }

    var latest *semver.Version
    for verStr := range pkgData.Versions {
        ver, err := semver.NewVersion(verStr)
        if err != nil {
            continue
        }
        if constraint.Check(ver) {
            if latest == nil || ver.GreaterThan(latest) {
                latest = ver
            }
        }
    }

    if latest == nil {
        log.Error("No version found matching range", nil, map[string]interface{}{
            "package": name,
            "range":   versionRange,
        })
        return "", fmt.Errorf("no version found for %s matching %s", name, versionRange)
    }

    log.Debug("Version resolved", map[string]interface{}{
        "package": name,
        "range":   versionRange,
        "version": latest.Original(),
    })
    return latest.Original(), nil
}