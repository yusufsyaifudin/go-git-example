package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/helper/polyfill"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/go-git/go-git/v5/utils/merkletrie"
	"github.com/stretchr/testify/assert"
)

var (
	ErrRootCommit = fmt.Errorf("error because this is root commit")
)

// main will implement git diff-tree
// go run diff-tree/main.go
// https://github.com/go-git/go-git/blob/v5.4.2/plumbing/object/difftree.go#L11-L17
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inMemStore := memory.NewStorage()
	workTree := polyfill.New(memfs.New())

	buf := &bytes.Buffer{}
	const gitPath = "https://github.com/yusufsyaifudin/benthos-sample.git"
	repo, err := git.CloneContext(ctx, inMemStore, workTree, &git.CloneOptions{
		URL:      gitPath,
		Progress: buf,
	})
	if err != nil {
		log.Fatalf("error plain clone: %s\n", err)
		return
	}

	fmt.Println(buf.String())

	// Here is the structure of commit in that project as of July 1st, 2022 when this program is written.
	// last commit: fe2c1dad736aeb8ffa996d777e4b6c7dc14e21d6 -> fe2c1da
	// mid  commit: 685438b58b9d75094fc15f97e29a416e6f9222a0 -> 685438b
	// root commit: 95e2b7dfabc5f43161a979a7e44dc0005dcfd467 -> 95e2b7d

	// Ensuring that diffTree output will always the same as we got in actual Git server repo.
	// In this case using Github, but any Git server must return the same output.
	t := Test()

	// you can compare here: https://github.com/yusufsyaifudin/benthos-sample/commit/fe2c1dad736aeb8ffa996d777e4b6c7dc14e21d6
	cidfe2c1da, err := diffTree(repo, "fe2c1dad736aeb8ffa996d777e4b6c7dc14e21d6")
	assert.EqualValues(t, []string{"README.md", "config/kafka/jaas.conf", "docker-compose-kafka.yaml"}, cidfe2c1da)
	assert.NoError(t, err)

	fmt.Println()

	// you can compare here: https://github.com/yusufsyaifudin/benthos-sample/commit/685438b58b9d75094fc15f97e29a416e6f9222a0
	cid685438b, err := diffTree(repo, "685438b58b9d75094fc15f97e29a416e6f9222a0")
	assert.EqualValues(t, []string{".gitignore", "golang/Makefile"}, cid685438b)
	assert.NoError(t, err)

	fmt.Println()

	// you can compare here: https://github.com/yusufsyaifudin/benthos-sample/commit/95e2b7dfabc5f43161a979a7e44dc0005dcfd467
	cid95e2b7d, err := diffTree(repo, "95e2b7dfabc5f43161a979a7e44dc0005dcfd467")
	assert.Empty(t, cid95e2b7d)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrRootCommit)

	return

}

// diffTree is helper function to get the changed file in specific commit. The command is as follows:
// git diff-tree --no-commit-id --name-only -r {selected-commit-id} {parent-of-selected-commit-id}
// But, we can omit the {parent-of-selected-commit-id} and the simplified version is:
// git diff-tree --no-commit-id --name-only -r {selected-commit-id}
//
// It should be noted that diff-tree won't work when looking at the root commit.
// This is expected behavior on git diff-tree. https://stackoverflow.com/a/424142
//
// You must pass long version of SHA-1 hash string to get the actual value. Otherwise, it will be Fatal error.
func diffTree(repo *git.Repository, commitID string) (files []string, err error) {
	files = make([]string, 0)

	selectedCommit, err := repo.CommitObject(plumbing.NewHash(commitID))
	if err != nil {
		err = fmt.Errorf("error get commit id %s: %w", commitID, err)
		return
	}

	selectedCommitTree, err := selectedCommit.Tree()
	if err != nil {
		err = fmt.Errorf("error get tree object of commit id %s: %w", commitID, err)
		return
	}

	parentCommit, err := selectedCommit.Parents().Next()
	if errors.Is(err, io.EOF) {
		err = fmt.Errorf("%w: commit id %s", ErrRootCommit, commitID)

		// expected behavior of git diff-tree
		fmt.Printf("no diff on commit %s because it is root commit\n", selectedCommit.Hash.String()[:7])
		fmt.Println(strings.Repeat("-", 30))
		return
	}

	if err != nil {
		err = fmt.Errorf("error parents iter object: %w", err)
		return
	}

	parentCommitTree, err := parentCommit.Tree()
	if err != nil {
		err = fmt.Errorf("error get parent commit of commit %s: %w", commitID, err)
		return
	}

	changes, err := object.DiffTree(parentCommitTree, selectedCommitTree)
	if err != nil {
		err = fmt.Errorf("error diff tree: %w", err)
		return
	}

	fmt.Printf("diff between %s vs %s\n", parentCommit.Hash.String()[:7], selectedCommit.Hash.String()[:7])
	fmt.Println(strings.Repeat("-", 30))

	// Below logic is extracted from library logic:
	// * To get file action: https://github.com/go-git/go-git/blob/v5.4.2/plumbing/object/change.go#L23-L40
	// * To get file name: https://github.com/go-git/go-git/blob/v5.4.2/plumbing/object/change.go#L98-L104
	// This actually what https://github.com/go-git/go-git/blob/v5.4.2/plumbing/object/change.go#L98-L104 and
	// https://github.com/go-git/go-git/blob/v5.4.2/plumbing/object/change.go#L75-L82 are doing.
	// Why we don't use library Action() function is because we need to get file name.
	// And as we see, the c.name() on L98-L104 is need by default using c.To, unless c.From is not empty.
	var empty object.ChangeEntry
	for _, c := range changes {
		if c.From == empty && c.To == empty {
			err = fmt.Errorf("malformed change: empty from and to")
			return
		}

		var action = merkletrie.Modify
		if c.From == empty {
			action = merkletrie.Insert
		}

		if c.To == empty {
			action = merkletrie.Delete
		}

		file := c.To
		if c.From != empty {
			file = c.From
		}

		insideDir := "[x]"
		if filepath.Dir(file.Name) != "." { // If the path is empty, Dir returns ".".
			insideDir = "[v]"
		}

		files = append(files, file.Name)
		fmt.Println(insideDir, file.Name, action)
	}

	return
}

// T mimics testing.T object to be used by assertion library.
type T struct{}

var _ assert.TestingT = (*T)(nil)

func Test() *T {
	return &T{}
}

func (t *T) Errorf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}
