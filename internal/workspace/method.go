package workspace

import "strings"

const tokenPunct = "!#$%&'*+-.^_`|~"

func ValidMethod(m string) bool {
	if m == "" {
		return false
	}
	for i := 0; i < len(m); i++ {
		c := m[i]
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !alnum && strings.IndexByte(tokenPunct, c) < 0 {
			return false
		}
	}
	return true
}
