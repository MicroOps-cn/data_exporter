package wrapper

const (
	PosRight = iota
	PosLeft
	PosCenter
)

func Limit[T any](s []T, limit int, hidePosition int, manySuffix ...T) []T {

	limit = limit - len(manySuffix)
	if len(s) > limit {
		var ret = make([]T, 0, limit+len(manySuffix))
		switch hidePosition {
		case PosRight:
			ret = append(ret, s[:limit]...)
			ret = append(ret, manySuffix...)
			return ret
		case PosLeft:
			ret = append(ret, manySuffix...)
			ret = append(ret, s[len(s)-limit:]...)
			return ret
		case PosCenter:
			ret = append(ret, s[:limit/2]...)
			ret = append(ret, manySuffix...)
			ret = append(ret, s[len(s)-(limit-limit/2):]...)
			return ret
		}
	}
	return s
}
