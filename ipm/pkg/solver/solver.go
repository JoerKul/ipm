package solver

import (
	"fmt"
	"ipm/pkg/log"
	"ipm/pkg/registry"
)

type DependencyNode struct {
	Name    string
	Version string
	Deps    map[string]string
}

type Solver struct {
	reg           registry.Registry
	nodes         map[string]*DependencyNode
	conflicts     []Conflict
	resolvedCache map[string]string // name:versionRange → resolvedVersion
}

type Conflict struct {
	Package    string
	Versions   []string
	Dependents []string
}

func NewSolver(reg registry.Registry) *Solver {
	return &Solver{
		reg:           reg,
		nodes:         make(map[string]*DependencyNode),
		resolvedCache: make(map[string]string),
	}
}

func (s *Solver) AddPackage(name, versionRange string) error {
	cacheKey := fmt.Sprintf("%s@%s", name, versionRange)
	if cachedVersion, ok := s.resolvedCache[cacheKey]; ok {
		log.Debug("Using cached resolved version", map[string]interface{}{
			"package": name,
			"range":   versionRange,
			"version": cachedVersion,
		})
		return s.addNode(name, cachedVersion)
	}

	version, err := s.reg.ResolveVersion(name, versionRange)
	if err != nil {
		return fmt.Errorf("failed to resolve %s@%s: %v", name, versionRange, err)
	}

	s.resolvedCache[cacheKey] = version
	return s.addNode(name, version)
}

func (s *Solver) addNode(name, version string) error {
	key := fmt.Sprintf("%s@%s", name, version)
	if _, ok := s.nodes[key]; ok {
		return nil
	}

	_, pkg, err := s.reg.FetchPackageTarball(name, version)
	if err != nil {
		return fmt.Errorf("failed to fetch %s@%s: %v", name, version, err)
	}

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