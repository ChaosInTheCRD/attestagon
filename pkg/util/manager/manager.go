package manager

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
)

// Interface is a manager that runs Added runnables in parallel.
type Interface interface {
	Add(...ctrlmanager.Runnable) error
	Start(context.Context) error
}

// manager is a simple manager that runs all runnables in parallel.
type manager struct {
	log       logr.Logger
	runnables []ctrlmanager.Runnable
	lock      sync.Mutex

	started bool
}

// New returns a new manager.
func New(log logr.Logger, rs ...ctrlmanager.Runnable) Interface {
	log = log.WithName("manager")
	return &manager{
		log:       log,
		runnables: rs,
	}
}

// Add adds a runnable to the manager to be run.
func (m *manager) Add(r ...ctrlmanager.Runnable) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.started {
		return errors.New("unable to add runnable to manager, manager is already started")
	}

	m.runnables = append(m.runnables, r...)

	return nil
}

// Start starts all runnables in the manager.
// When one of the runnables returns an error, all runnables are stopped.
// Waits for all runnables to stop before returning.
func (m *manager) Start(ctx context.Context) error {
	m.lock.Lock()
	if m.started {
		m.lock.Unlock()
		return errors.New("unable to start manager, manager is already started")
	}
	m.started = true
	m.lock.Unlock()

	m.log.Info("starting manager")

	var (
		wg   sync.WaitGroup
		errs []string
	)

	wg.Add(len(m.runnables))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, r := range m.runnables {
		go func(r ctrlmanager.Runnable) {
			defer wg.Done()
			defer cancel()
			err := r.Start(ctx)
			if err != nil {
				m.lock.Lock()
				errs = append(errs, err.Error())
				m.lock.Unlock()
			}
		}(r)
	}

	wg.Wait()

	m.log.Info("closing manager")

	if len(errs) > 0 {
		return errors.New("error running manager: " + strings.Join(errs, "; "))
	}

	return nil
}
