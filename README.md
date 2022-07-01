# go-git example

This repo is just another example to implement low level (plumbing) and high level (porcelain) Git, using purely Golang 
using https://github.com/go-git/go-git/tree/v5.4.2

## Current example includes:

* [x] [`git diff-tree`](/diff-tree) - to proof that we can get changes/differences only using specific commit ID (similar like we see when we click commit ID on Github/Gitlab)


## How to run

Please note that some variable is hardcoded to make it easier to re-run the program.

* Clone repo and `go mod download`
* Run `go run` into specific file `main.go`, for example: `go run diff-tree/main.go`
