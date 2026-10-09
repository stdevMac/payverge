# strip-log-noise.awk
#
# Consumed by `make bench-ci`. Cleans up `go test -bench` output so benchstat
# can parse every bench, even when log.Println calls from RBAC bootstrap (or
# anywhere else) land on the same line as a bench name.
#
# Input (excerpt):
#   BenchmarkAuthMiddleware-8   	2026/05/23 13:37:21 RBAC data initialization completed successfully
#   2026/05/23 13:37:26 RBAC data initialization completed successfully
#     244461	      4969 ns/op	   10212 B/op	      85 allocs/op
#
# Output:
#   BenchmarkAuthMiddleware-8   	  244461	      4969 ns/op	   10212 B/op	      85 allocs/op
#
# Rules:
#   1. Drop standalone log lines: leading whitespace + YYYY/MM/DD HH:MM:SS ...
#   2. For a bench-name line followed by a log timestamp on the same line,
#      strip everything from the timestamp onward, hold the bench name, and
#      glue it to the next iteration-result line (which starts with whitespace
#      + digits + tab).
#   3. Pass non-bench lines (goos, pkg, PASS, etc.) through unchanged.

BEGIN { pending = "" }

# Standalone log line: drop it.
/^[[:space:]]*[0-9]{4}\/[0-9]{2}\/[0-9]{2} [0-9:]+ / {
    next
}

# Bench-name line with an attached log timestamp. Hold the cleaned prefix
# until the next result line lands.
/^Benchmark[^[:space:]]+[[:space:]]+[0-9]{4}\/[0-9]{2}\/[0-9]{2} [0-9:]+ / {
    line = $0
    sub(/[[:space:]]+[0-9]{4}\/[0-9]{2}\/[0-9]{2} [0-9:]+ .*$/, "", line)
    pending = line
    next
}

# Result line. If we are holding a pending bench name, attach this line's
# numeric tail to it. Otherwise pass through.
/^[[:space:]]+[0-9]+[[:space:]]+/ {
    if (pending != "") {
        print pending "\t" $0
        pending = ""
        next
    }
}

# Default: pass through. If we accidentally swallowed a bench name without
# finding a result line (shouldn't happen for go test -bench output), flush
# it on the next non-empty line so we never lose data silently.
{
    if (pending != "" && $0 !~ /^$/) {
        print pending
        pending = ""
    }
    print
}

END {
    if (pending != "") {
        print pending
    }
}
