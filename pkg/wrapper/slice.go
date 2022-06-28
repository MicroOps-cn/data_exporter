// Copyright 2021 MicroOps
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
