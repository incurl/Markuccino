// Package vcs isolates the version-control implementation from the
// writing domain.
//
// The rest of the application depends on the Repository interface, never
// on go-git. That is deliberate: the Git dependency stays behind one
// boundary (repository init, commit, history, read, restore), so the
// implementation can be replaced — for example with a system `git` CLI
// integration — without touching the domain model.
//
// The shipped implementation is pure Go, so the desktop binary stays
// CGO-free and statically buildable.
package vcs

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoCommits is returned when a repository exists but has no commit
// yet. It is not a failure of the caller: an empty repository is a valid
// intermediate state.
var ErrNoCommits = errors.New("the repository has no commits yet")

// ErrNotFound is returned when a path or a revision does not exist in
// the repository.
var ErrNotFound = errors.New("not found in the repository")

// Commit is one revision of the project.
type Commit struct {
	// Hash is the full commit hash.
	Hash string `json:"hash"`
	// Parent is the first parent hash, empty for the initial commit.
	Parent string `json:"parent"`
	// Message is the commit message.
	Message string `json:"message"`
	// Author is the name of the author.
	Author string `json:"author"`
	// When is the commit time, in RFC 3339.
	When string `json:"when"`
	// Files lists the paths the commit changed.
	Files []string `json:"files"`
	// Created reports whether the commit was created by the call that
	// returned it. A commit made against an unchanged worktree returns
	// the previous revision with Created false, so a snapshot can record
	// the revision without pretending it is new.
	Created bool `json:"created"`
}

// Repository is the version-control surface the writing domain uses.
type Repository interface {
	// Root returns the working-tree directory.
	Root() string

	// Head returns the current commit hash. It returns ErrNoCommits when
	// the repository is empty.
	Head() (string, error)

	// Commit records the current state of the working tree. When nothing
	// changed it returns the previous revision with Created false.
	Commit(ctx context.Context, message string) (Commit, error)

	// History returns the revisions, newest first. A limit of zero means
	// no limit.
	History(ctx context.Context, limit int) ([]Commit, error)

	// Get returns one revision.
	Get(ctx context.Context, hash string) (Commit, error)

	// ReadFile returns the content of path at commit. An empty commit
	// means HEAD.
	ReadFile(ctx context.Context, commit, path string) (string, error)

	// ListFiles lists the files in the tree at commit. An empty commit
	// means HEAD.
	ListFiles(ctx context.Context, commit string) ([]string, error)

	// RestoreFile writes the content of path at commit back into the
	// working tree. It does not commit: the caller decides the message
	// and records the snapshot.
	RestoreFile(ctx context.Context, commit, path string) error

	// HasChanges reports whether the working tree differs from HEAD.
	HasChanges() (bool, error)
}

// ValidatePath rejects a path that could leave the working tree. The
// repository never receives a path from the user directly — it comes
// from a PocketBase record — but a boundary check costs nothing and
// stops a malformed record from writing outside the project.
func ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("vcs: empty path")
	}
	if len(path) > 512 {
		return fmt.Errorf("vcs: path is too long")
	}
	for _, r := range path {
		switch r {
		case '\\':
			return fmt.Errorf("vcs: path %q contains a backslash", path)
		}
	}
	if path[0] == '/' {
		return fmt.Errorf("vcs: path %q is absolute", path)
	}
	// filepath.Clean would also accept "a/../b"; reject any parent
	// segment outright.
	for _, segment := range splitPath(path) {
		if segment == ".." {
			return fmt.Errorf("vcs: path %q escapes the working tree", path)
		}
	}
	return nil
}

// splitPath splits on the forward slash, which is the separator Git uses
// regardless of the platform.
func splitPath(path string) []string {
	segments := make([]string, 0, 4)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			segments = append(segments, path[start:i])
			start = i + 1
		}
	}
	return segments
}
