package types

type Package struct {
    Name    string
    Version string
    Deps    map[string]string // z. B. "statuses": "~1.3.1"
}