package corpus

/* A block comment that
   spans several lines
   before any code. */

const banner = `raw string
spanning two lines`

func Hello(name string) string {
	return "hello, " + name // trailing comment
}
