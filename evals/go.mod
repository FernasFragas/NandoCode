// Module boundary for eval fixtures: keeps golden expected/ files out of the
// root module so go build/vet/test, golangci-lint and gosec skip them.
module github.com/FernasFragas/Nandocode/evals

go 1.26.2
