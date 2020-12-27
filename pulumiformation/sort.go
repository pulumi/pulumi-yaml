package pulumiformation

import "github.com/pkg/errors"

func topologicallySortedResources(t *Template) ([]string, error) {
	var sorted []string               // will hold the sorted vertices.
	visiting := make(map[string]bool) // temporary entries to detect cycles.
	visited := make(map[string]bool)  // entries to avoid visiting the same node twice.

	// Precompute dependencies for each resource
	dependencies := make(map[string][]string)
	for rname, r := range t.Resources {
		resnames, err := GetResourceDependencies(r)
		if err != nil {
			return nil, err
		}
		dependencies[rname] = resnames
	}

	// Depth-first visit each node
	var visit func(name string) error
	visit = func(name string) error {
		if visiting[name] {
			return errors.Errorf("circular dependency of resource '%s' transitively on itself", name)
		}
		if !visited[name] {
			visiting[name] = true
			for _, mname := range dependencies[name] {
				if err := visit(mname); err != nil {
					return err
				}
			}
			visited[name] = true
			visiting[name] = false
			sorted = append(sorted, name)
		}
		return nil
	}

	// Repeatedly visit the first unvisited unode until none are left
	for {
		progress := false
		for rname := range t.Resources {
			if !visited[rname] {
				err := visit(rname)
				if err != nil {
					return nil, err
				}
				progress = true
				break
			}
		}
		if !progress {
			break
		}
	}
	return sorted, nil
}
