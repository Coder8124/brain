package main

import (
	"github.com/pragun/brain/internal/action"
	"github.com/pragun/brain/internal/bizagent"
)

// Outbound-action approval — the trust loop's higher-stakes half, in the app.
// Pending actions carry a full preview; approving is the only path that runs the
// effect, and it goes through the same gate the CLI uses.

func (a *App) PendingActions() ([]action.Action, error) {
	ix, err := a.open()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if err := action.Init(ix.DB); err != nil {
		return nil, err
	}
	return action.List(ix.DB, action.Pending)
}

func (a *App) ApproveAction(id int64) (string, error) {
	ix, err := a.open()
	if err != nil {
		return "", err
	}
	defer ix.Close()
	if err := action.Init(ix.DB); err != nil {
		return "", err
	}
	// Executors must be registered before an action can run.
	bizagent.RegisterDefaultExecutors(a.vault)
	return action.Approve(ix.DB, id)
}

func (a *App) RejectAction(id int64) error {
	ix, err := a.open()
	if err != nil {
		return err
	}
	defer ix.Close()
	if err := action.Init(ix.DB); err != nil {
		return err
	}
	return action.Reject(ix.DB, id)
}
