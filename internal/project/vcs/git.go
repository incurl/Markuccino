package vcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// AuthorName and AuthorEmail identify the commits the application makes.
//
// They are constants rather than configuration: the repository is local
// to one project and one user, and the commit metadata the UI shows is
// the timestamp and the message. A future "author" setting belongs in the
// project settings, not in this package.
const (
	AuthorName  = "Markuccino"
	AuthorEmail = "markuccino@localhost"
)

// Repo is the go-git implementation of Repository.
//
// It is the only file in the application that imports go-git. Keeping
// that import here is what makes the dependency replaceable.
type Repo struct {
	root string
	repo *git.Repository
}

// Open opens the repository at root, initialising it when it does not
// exist yet.
//
// A project always has a repository: CreateProject calls Open before it
// writes the first file, so the first snapshot has somewhere to go.
func Open(root string) (*Repo, error) {
	if root == "" {
		return nil, fmt.Errorf("vcs: empty repository root")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("vcs: create repository root: %w", err)
	}

	repository, err := git.PlainOpen(root)
	if err == nil {
		return &Repo{root: root, repo: repository}, nil
	}
	if !errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, fmt.Errorf("vcs: open repository: %w", err)
	}

	repository, err = git.PlainInitWithOptions(root, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.Main},
	})
	if err != nil {
		return nil, fmt.Errorf("vcs: initialise repository: %w", err)
	}
	return &Repo{root: root, repo: repository}, nil
}

// Root returns the working-tree directory.
func (r *Repo) Root() string { return r.root }

// worktree returns the working tree.
func (r *Repo) worktree() (*git.Worktree, error) {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("vcs: open worktree: %w", err)
	}
	return worktree, nil
}

// Head returns the current commit hash.
func (r *Repo) Head() (string, error) {
	ref, err := r.repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", ErrNoCommits
		}
		return "", fmt.Errorf("vcs: read HEAD: %w", err)
	}
	return ref.Hash().String(), nil
}

// Commit records the working tree.
func (r *Repo) Commit(ctx context.Context, message string) (Commit, error) {
	if err := ctx.Err(); err != nil {
		return Commit{}, err
	}
	if strings.TrimSpace(message) == "" {
		message = "Checkpoint"
	}

	worktree, err := r.worktree()
	if err != nil {
		return Commit{}, err
	}

	// AddWithOptions with All stages modifications, deletions and new
	// files in one pass. Without it a new document would never reach a
	// commit, because go-git's CommitOptions.All ignores untracked files.
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return Commit{}, fmt.Errorf("vcs: stage the working tree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return Commit{}, fmt.Errorf("vcs: read the working tree status: %w", err)
	}
	if status.IsClean() {
		// Nothing changed. Return the current revision so the caller can
		// still record which revision it is looking at.
		head, headErr := r.Head()
		if headErr != nil {
			if errors.Is(headErr, ErrNoCommits) {
				// An empty repository with an empty tree: create the
				// initial commit so the project has a base revision.
				return r.writeCommit(worktree, message)
			}
			return Commit{}, headErr
		}
		commit, commitErr := r.commitByHash(head)
		if commitErr != nil {
			return Commit{}, commitErr
		}
		commit.Created = false
		return commit, nil
	}

	return r.writeCommit(worktree, message)
}

// writeCommit creates the commit and converts it to the public shape.
func (r *Repo) writeCommit(worktree *git.Worktree, message string) (Commit, error) {
	signature := &object.Signature{
		Name:  AuthorName,
		Email: AuthorEmail,
		When:  time.Now(),
	}
	hash, err := worktree.Commit(message, &git.CommitOptions{
		Author:            signature,
		Committer:         signature,
		AllowEmptyCommits: true,
	})
	if err != nil {
		return Commit{}, fmt.Errorf("vcs: create the commit: %w", err)
	}
	commit, err := r.commitByHash(hash.String())
	if err != nil {
		return Commit{}, err
	}
	commit.Created = true
	return commit, nil
}

// commitByHash loads one commit and describes it.
func (r *Repo) commitByHash(hash string) (Commit, error) {
	commit, err := r.repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return Commit{}, fmt.Errorf("vcs: load commit %s: %w", short(hash), err)
	}
	return describeCommit(commit), nil
}

// describeCommit converts a go-git commit into the public shape.
func describeCommit(commit *object.Commit) Commit {
	result := Commit{
		Hash:    commit.Hash.String(),
		Message: strings.TrimSpace(commit.Message),
		Author:  commit.Author.Name,
		When:    commit.Author.When.UTC().Format(time.RFC3339),
		Files:   []string{},
	}
	if commit.NumParents() > 0 {
		if parent, err := commit.Parent(0); err == nil {
			result.Parent = parent.Hash.String()
		}
	}
	if stats, err := commit.Stats(); err == nil {
		for _, stat := range stats {
			result.Files = append(result.Files, stat.Name)
		}
	}
	return result
}

// History returns the revisions, newest first.
func (r *Repo) History(ctx context.Context, limit int) ([]Commit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	head, err := r.Head()
	if err != nil {
		if errors.Is(err, ErrNoCommits) {
			return []Commit{}, nil
		}
		return nil, err
	}

	iterator, err := r.repo.Log(&git.LogOptions{
		From:  plumbing.NewHash(head),
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("vcs: read the history: %w", err)
	}
	defer iterator.Close()

	commits := make([]Commit, 0, 16)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		commit, nextErr := iterator.Next()
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("vcs: read the history: %w", nextErr)
		}
		commits = append(commits, describeCommit(commit))
		if limit > 0 && len(commits) >= limit {
			break
		}
	}
	return commits, nil
}

// Get returns one revision.
func (r *Repo) Get(ctx context.Context, hash string) (Commit, error) {
	if err := ctx.Err(); err != nil {
		return Commit{}, err
	}
	if strings.TrimSpace(hash) == "" {
		return Commit{}, fmt.Errorf("vcs: a revision hash is required")
	}
	return r.commitByHash(hash)
}

// ReadFile returns the content of path at commit.
func (r *Repo) ReadFile(ctx context.Context, commit, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := ValidatePath(path); err != nil {
		return "", err
	}
	tree, err := r.treeAt(commit)
	if err != nil {
		return "", err
	}
	file, err := tree.File(path)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return "", fmt.Errorf("%w: %s at %s", ErrNotFound, path, short(commit))
		}
		return "", fmt.Errorf("vcs: read %s: %w", path, err)
	}
	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("vcs: read %s: %w", path, err)
	}
	return content, nil
}

// ListFiles lists the files in the tree at commit.
func (r *Repo) ListFiles(ctx context.Context, commit string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tree, err := r.treeAt(commit)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, 32)
	iterator := tree.Files()
	defer iterator.Close()
	for {
		file, nextErr := iterator.Next()
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("vcs: list the tree: %w", nextErr)
		}
		files = append(files, file.Name)
	}
	return files, nil
}

// treeAt resolves a commit reference and returns its tree.
func (r *Repo) treeAt(commit string) (*object.Tree, error) {
	hash, err := r.resolve(commit)
	if err != nil {
		return nil, err
	}
	loaded, err := r.repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("vcs: load commit %s: %w", short(hash.String()), err)
	}
	tree, err := loaded.Tree()
	if err != nil {
		return nil, fmt.Errorf("vcs: load the tree of %s: %w", short(hash.String()), err)
	}
	return tree, nil
}

// resolve turns a revision string into a hash. Empty means HEAD.
func (r *Repo) resolve(revision string) (plumbing.Hash, error) {
	if revision == "" {
		head, err := r.Head()
		if err != nil {
			return plumbing.ZeroHash, err
		}
		return plumbing.NewHash(head), nil
	}
	hash, err := r.repo.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("vcs: resolve revision %q: %w", revision, err)
	}
	return *hash, nil
}

// RestoreFile writes the content of path at commit into the working
// tree. It does not commit: the caller decides the message and records
// the snapshot.
func (r *Repo) RestoreFile(ctx context.Context, commit, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	content, err := r.ReadFile(ctx, commit, path)
	if err != nil {
		return err
	}
	target := filepath.Join(r.root, filepath.FromSlash(path))
	// Defence in depth: ReadFile validated the path, and this check
	// makes sure the joined path is still inside the root.
	relative, err := filepath.Rel(r.root, target)
	if err != nil || strings.HasPrefix(relative, "..") {
		return fmt.Errorf("vcs: path %q escapes the repository", path)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("vcs: create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		return fmt.Errorf("vcs: restore %s: %w", path, err)
	}
	return nil
}

// HasChanges reports whether the working tree differs from HEAD.
func (r *Repo) HasChanges() (bool, error) {
	worktree, err := r.worktree()
	if err != nil {
		return false, err
	}
	status, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf("vcs: read the working tree status: %w", err)
	}
	return !status.IsClean(), nil
}

// short returns the first seven characters of a hash for messages.
func short(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
}
