test:
    #!/usr/bin/env fish
    for test in (go test -list .)
        go test -run=$test -v
    end

man:
    go run -tags mangen . > nak.1
