package caddy

import "fmt"

func Snippet(host, upstream string) string {
	return fmt.Sprintf(`%s {
	reverse_proxy %s
}
`, host, upstream)
}
