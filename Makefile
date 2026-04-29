.PHONY: demo-failing-hook test

test:
	go test ./...

demo-failing-hook:
	examples/failing-hook/run.sh
