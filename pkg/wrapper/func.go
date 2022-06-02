package wrapper

func M[T any](ret T, err error) T {
	if err != nil {
		panic(err)
	}
	return ret
}

//func Must[T any](ret T, err error) T {
//	return M[T](ret, err)
//}
