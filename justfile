test:
    #!/usr/bin/env fish
    for test in (go test -list .)
        go test -run=$test -v
    end
