package solver

import (
	"fmt"
	"ipm/pkg/registry"
)

type DependencyNode struct {
    Name    string
    Version string
    Deps    map[string]string
}

type Solver struct {
    reg       registry.Registry
    nodes     map[string]*DependencyNode // name:version → Node
    conflicts []Conflict
}

type Conflict struct {
    Package    string
    Versions   []string
    Dependents []string
}

func NewSolver(reg registry.Registry) *Solver {
    return &Solver{
        reg:   reg,
        nodes: make(map[string]*DependencyNode),
    }
}

func (s *Solver) AddPackage(name, versionRange string) error {
    // Löse Version auf
    version, err := s.reg.ResolveVersion(name, versionRange)
    if err != nil {
        return fmt.Errorf("failed to resolve %s@%s: %v", name, versionRange, err)
    }

    key := fmt.Sprintf("%s@%s", name, version)
    if _, ok := s.nodes[key]; ok {
        // Bereits hinzugefügt, Abhängigkeiten müssen konsistent sein
        return nil
    }

    // Hole Paketdetails
    _, pkg, err := s.reg.FetchPackageTarball(name, version)
    if err != nil {
        return fmt.Errorf("failed to fetch %s@%s: %v", name, version, err)
    }

    // Prüfe auf Konflikte mit bestehenden Versionen
    for existingKey, node := range s.nodes {
        if node.Name == name && node.Version != version {
            s.conflicts = append(s.conflicts, Conflict{
                Package:    name,
                Versions:   []string{node.Version, version},
                Dependents: []string{existingKey, key},
            })
        }
    }

    s.nodes[key] = &DependencyNode{
        Name:    name,
        Version: version,
        Deps:    pkg.Deps,
    }

    // Rekursiv Abhängigkeiten hinzufügen
    for depName, depVersion := range pkg.Deps {
        if err := s.AddPackage(depName, depVersion); err != nil {
            return err
        }
    }

    return nil
}

func (s *Solver) HasConflicts() bool {
    return len(s.conflicts) > 0
}

func (s *Solver) GetConflicts() []Conflict {
    return s.conflicts
}