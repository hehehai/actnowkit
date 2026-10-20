package licence

import (
	"strconv"
	"strings"
)

// CompareVersions compares semver-like versions ("1.2.3", "1.2.3-beta.2")
// as actnow.dev does: negative when a < b. A pre-release sorts before its
// release; numeric identifiers compare as numbers; a leading "v" and build
// metadata are ignored.
func CompareVersions(a, b string) int {
	coreA, preA := splitVersion(a)
	coreB, preB := splitVersion(b)
	pa, pb := strings.Split(coreA, "."), strings.Split(coreB, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		if d := num(at(pa, i)) - num(at(pb, i)); d != 0 {
			return sign(d)
		}
	}
	switch {
	case preA == "" && preB == "":
		return 0
	case preA == "":
		return 1
	case preB == "":
		return -1
	}
	ia, ib := strings.Split(preA, "."), strings.Split(preB, ".")
	for i := 0; i < max(len(ia), len(ib)); i++ {
		if i >= len(ia) {
			return -1
		}
		if i >= len(ib) {
			return 1
		}
		x, y := ia[i], ib[i]
		nx, ex := strconv.Atoi(x)
		ny, ey := strconv.Atoi(y)
		switch {
		case ex == nil && ey == nil:
			if nx != ny {
				return sign(nx - ny)
			}
		case ex == nil:
			return -1
		case ey == nil:
			return 1
		case x != y:
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// WhitelistBlocked tells whether early-access (whitelist) licences stop at
// this version: version ≥ paidSince. An empty paidSince blocks nothing.
func WhitelistBlocked(version, paidSince string) bool {
	return paidSince != "" && CompareVersions(version, paidSince) >= 0
}

func splitVersion(v string) (core, pre string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "+")
	core, pre, _ = strings.Cut(v, "-")
	return core, pre
}

func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "0"
}

func num(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
