package execute

import (
	"fmt"
	"strings"
)

func formatOutput(outString string) string {
	var result []string = make([]string, 0, 1)
	for partial := range tokenizer(outString) {
		result = append(result, partial)
	}
	return strings.Join(result, "")
}

func tokenizer(s string) chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		i, j := 0, 0
		for ; j < len(s); j++ {
			r := s[j]
			if r < ' ' || r == byte(127) {
				if j-i > 1 {
					ch <- s[i:j]
				}
				ch <- fmt.Sprintf("\\x%02x", uint(r))
				i = j + 1
			}
		}
		ch <- s[i:]
	}()
	return ch
}
