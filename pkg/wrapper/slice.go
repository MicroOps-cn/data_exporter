package wrapper

func Limit[T any](s []T, limit int, manySuffix ...T) []T {
	if len(s) > limit {
		return append(s[:limit], manySuffix...)
	}
	return s
}
