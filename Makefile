.PHONY: demo-cdi-like-hook demo-failing-hook test

test:
	go test ./...

demo-failing-hook:
	examples/failing-hook/run.sh

demo-cdi-like-hook:
	examples/cdi-like-hook/run.sh
