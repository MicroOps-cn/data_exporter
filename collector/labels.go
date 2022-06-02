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

package collector

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v3"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	relabelTarget        = regexp.MustCompile(`^(?:(?:[a-zA-Z_]|\$(?:\{\w+\}|\w+))+\w*)+$`)
	DefaultRelabelConfig = RelabelConfig{
		Action:      Replace,
		Separator:   ";",
		Regex:       MustNewRegexp("(.*)"),
		Replacement: "$1",
	}
	seps = []byte{'\xff'}
)

const (
	LabelMetricName       = "__name__"
	LabelMetricNamespace  = "__namespace__"
	LabelMetricSubsystem  = "__subsystem__"
	LabelMetricHelp       = "__help__"
	LabelMetricTime       = "__time__"
	LabelMetricTimeFormat = "__time_format__"
	LabelMetricValue      = "__value__"
	LabelMetricBuckets    = "__buckets__"
	//LabelMetricDescribe   = "__describe__"
	labelSep = '\xfe'
)

func isInnerLabel(label Label) bool {
	return strings.HasPrefix(label.Name, "__")
}

type Regexp struct {
	*regexp.Regexp
	original string
}

func MustNewRegexp(s string) Regexp {
	re, err := NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}

func NewRegexp(s string) (Regexp, error) {
	regex, err := regexp.Compile("^(?:" + s + ")$")
	return Regexp{
		Regexp:   regex,
		original: s,
	}, err
}

func (re Regexp) MarshalYAML() (interface{}, error) {
	if re.original != "" {
		return re.original, nil
	}
	return nil, nil
}

func (re *Regexp) UnmarshalYAML(value *yaml.Node) error {
	var regex string
	if err := value.Decode(&regex); err != nil {
		return err
	} else if len(regex) != 0 {
		if r, err := NewRegexp(regex); err != nil {
			return err
		} else {
			*re = r
		}
	}
	return nil
}

type Action string

const (
	// Replace performs a regex replacement.
	Replace Action = "replace"
	// TemplateExecute performs a template execute replacement.
	TemplateExecute Action = "templexec"
	// Keep drops targets for which the input does not match the regex.
	Keep Action = "keep"
	// Drop drops targets for which the input does match the regex.
	Drop Action = "drop"
	// HashMod sets a label to the modulus of a hash of labels.
	HashMod Action = "hashmod"
	// LabelMap copies labels to other labelnames based on a regex.
	LabelMap Action = "labelmap"
	// LabelDrop drops any label matching the regex.
	LabelDrop Action = "labeldrop"
	// LabelKeep drops any label not matching the regex.
	LabelKeep Action = "labelkeep"
)

type RelabelConfig struct {
	// A list of labels from which values are taken and concatenated
	// with the configured separator in order.
	SourceLabels model.LabelNames `yaml:"source_labels,flow,omitempty"`
	// Separator is the string between concatenated values from the source labels.
	Separator string `yaml:"separator,omitempty"`
	// Regex against which the concatenation is matched.
	Regex Regexp `yaml:"regex,omitempty"`
	// Template perform the replacement according to the template
	Template Template `yaml:"template,omitempty"`
	// Modulus to take of the hash of concatenated values from the source labels.
	Modulus uint64 `yaml:"modulus,omitempty"`
	// TargetLabel is the label to which the resulting string is written in a replacement.
	// Regexp interpolation is allowed for the replace action.
	TargetLabel string `yaml:"target_label,omitempty"`
	// Replacement is the regex replacement pattern to be used.
	Replacement string `yaml:"replacement,omitempty"`
	// Action is the action to be performed for the relabeling.
	Action Action `yaml:"action,omitempty"`
}

type RelabelConfigs []*RelabelConfig

func (rcs RelabelConfigs) String() string {
	var ret []map[string]interface{}
	buf, err := yaml.Marshal(rcs)
	if err != nil {
		return fmt.Sprintf("<Unknown Error: %s>", err)
	}

	err = yaml.Unmarshal(buf, &ret)
	if err != nil {
		return fmt.Sprintf("<Unknown Error: %s>", err)
	}
	marshal, err := json.Marshal(ret)
	if err != nil {
		return fmt.Sprintf("<Unknown Error: %s>", err)
	}
	return string(marshal)
}

func (c *RelabelConfig) UnmarshalYAML(value *yaml.Node) error {
	*c = DefaultRelabelConfig
	type plain RelabelConfig
	if err := value.Decode((*plain)(c)); err != nil {
		return err
	}
	if c.Regex.Regexp == nil {
		c.Regex = MustNewRegexp("")
	}
	if c.Template.Template == nil {
		c.Template = MustNewTemplate("", "")
	}
	if (c.Action == TemplateExecute) && c.Template.original == "" {
		return fmt.Errorf("relabel configuration for %s action requires 'template' value", c.Action)
	}
	if c.Modulus == 0 && c.Action == HashMod {
		return fmt.Errorf("relabel configuration for hashmod requires non-zero modulus")
	}
	if (c.Action == Replace || c.Action == HashMod || c.Action == TemplateExecute) && c.TargetLabel == "" {
		return fmt.Errorf("relabel configuration for %s action requires 'target_label' value", c.Action)
	}
	if c.Action == Replace && !relabelTarget.MatchString(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}
	if c.Action == LabelMap && !relabelTarget.MatchString(c.Replacement) {
		return fmt.Errorf("%q is invalid 'replacement' for %s action", c.Replacement, c.Action)
	}
	if c.Action == HashMod && !model.LabelName(c.TargetLabel).IsValid() {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}

	if c.Action == LabelDrop || c.Action == LabelKeep {
		if c.SourceLabels != nil ||
			c.TargetLabel != DefaultRelabelConfig.TargetLabel ||
			c.Modulus != DefaultRelabelConfig.Modulus ||
			c.Separator != DefaultRelabelConfig.Separator ||
			c.Replacement != DefaultRelabelConfig.Replacement {
			return fmt.Errorf("%s action requires only 'regex', and no other fields", c.Action)
		}
	}

	return nil

}

// Process returns a relabeled copy of the given label set. The relabel configurations
// are applied in order of input.
// If a label set is dropped, nil is returned.
// May return the input labelSet modified.
func (rcs RelabelConfigs) Process(labels Labels) (Labels, error) {
	var err error
	for _, rc := range rcs {
		labels, err = relabel(labels, rc)
		if labels == nil || err != nil {
			return nil, err
		}
	}
	return labels, nil
}

func relabel(lset Labels, cfg *RelabelConfig) (Labels, error) {
	values := make([]string, 0, len(cfg.SourceLabels))
	for _, ln := range cfg.SourceLabels {
		values = append(values, lset.Get(string(ln)))
	}
	val := strings.Join(values, cfg.Separator)

	lb := NewBuilder(lset)

	switch cfg.Action {
	case Drop:
		if cfg.Regex.MatchString(val) {
			return nil, nil
		}
	case Keep:
		if !cfg.Regex.MatchString(val) {
			return nil, nil
		}
	case TemplateExecute:
		newVal, err := cfg.Template.Execute(val)
		if err != nil {
			return nil, fmt.Errorf("faile to execute template: %s,err: %s", cfg.Template.original, err)
		}
		lb.Set(cfg.TargetLabel, string(newVal))
	case Replace:
		indexes := cfg.Regex.FindStringSubmatchIndex(val)
		// If there is no match no replacement must take place.
		if indexes == nil {
			break
		}
		target := model.LabelName(cfg.Regex.ExpandString([]byte{}, cfg.TargetLabel, val, indexes))
		if !target.IsValid() {
			lb.Del(cfg.TargetLabel)
			break
		}
		res := cfg.Regex.ExpandString([]byte{}, cfg.Replacement, val, indexes)
		if len(res) == 0 {
			lb.Del(cfg.TargetLabel)
			break
		}
		lb.Set(string(target), string(res))
	case HashMod:
		mod := sum64(md5.Sum([]byte(val))) % cfg.Modulus
		lb.Set(cfg.TargetLabel, fmt.Sprintf("%d", mod))
	case LabelMap:
		for _, l := range lset {
			if cfg.Regex.MatchString(l.Name) {
				res := cfg.Regex.ReplaceAllString(l.Name, cfg.Replacement)
				lb.Set(res, l.Value)
			}
		}
	case LabelDrop:
		for _, l := range lset {
			if cfg.Regex.MatchString(l.Name) {
				lb.Del(l.Name)
			}
		}
	case LabelKeep:
		for _, l := range lset {
			if !cfg.Regex.MatchString(l.Name) {
				lb.Del(l.Name)
			}
		}
	default:
		panic(fmt.Errorf("relabel: unknown relabel action type %q", cfg.Action))
	}

	return lb.Labels(), nil
}

// sum64 sums the md5 hash to an uint64.
func sum64(hash [md5.Size]byte) uint64 {
	var s uint64

	for i, b := range hash {
		shift := uint64((md5.Size - i - 1) * 8)

		s |= uint64(b) << shift
	}
	return s
}

// Builder allows modifying Labels.
type Builder struct {
	base Labels
	del  []string
	add  []Label
}

// NewBuilder returns a new LabelsBuilder.
func NewBuilder(base Labels) *Builder {
	b := &Builder{
		del: make([]string, 0, 5),
		add: make([]Label, 0, 5),
	}
	b.Reset(base)
	return b
}

// Reset clears all current state for the builder.
func (b *Builder) Reset(base Labels) {
	b.base = base
	b.del = b.del[:0]
	b.add = b.add[:0]
	for _, l := range b.base {
		if l.Value == "" {
			b.del = append(b.del, l.Name)
		}
	}
}

// Del deletes the label of the given name.
func (b *Builder) Del(ns ...string) *Builder {
	for _, n := range ns {
		for i, a := range b.add {
			if a.Name == n {
				b.add = append(b.add[:i], b.add[i+1:]...)
			}
		}
		b.del = append(b.del, n)
	}
	return b
}

// Set the name/value pair as a label.
func (b *Builder) Set(n, v string) *Builder {
	if v == "" {
		// Empty labels are the same as missing labels.
		return b.Del(n)
	}
	for i, a := range b.add {
		if a.Name == n {
			b.add[i].Value = v
			return b
		}
	}
	b.add = append(b.add, Label{Name: n, Value: v})

	return b
}

// Labels returns the labels from the builder. If no modifications
// were made, the original labels are returned.
func (b *Builder) Labels() Labels {
	if len(b.del) == 0 && len(b.add) == 0 {
		return b.base
	}

	// In the general case, labels are removed, modified or moved
	// rather than added.
	res := make(Labels, 0, len(b.base))
Outer:
	for _, l := range b.base {
		for _, n := range b.del {
			if l.Name == n {
				continue Outer
			}
		}
		for _, la := range b.add {
			if l.Name == la.Name {
				continue Outer
			}
		}
		res = append(res, l)
	}
	res = append(res, b.add...)
	sort.Sort(res)

	return res
}

type Label struct {
	Name, Value string
}

func (ls Labels) Len() int           { return len(ls) }
func (ls Labels) Swap(i, j int)      { ls[i], ls[j] = ls[j], ls[i] }
func (ls Labels) Less(i, j int) bool { return ls[i].Name < ls[j].Name }

func (ls Labels) String() string {
	var b bytes.Buffer

	b.WriteByte('{')
	for i, l := range ls {
		if i > 0 {
			b.WriteByte(',')
			b.WriteByte(' ')
		}
		b.WriteString(l.Name)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(l.Value))
	}
	b.WriteByte('}')
	return b.String()
}

// Bytes returns ls as a byte slice.
// It uses an byte invalid character as a separator and so should not be used for printing.
func (ls Labels) Bytes(buf []byte) []byte {
	b := bytes.NewBuffer(buf[:0])
	b.WriteByte(labelSep)
	for i, l := range ls {
		if i > 0 {
			b.WriteByte(seps[0])
		}
		b.WriteString(l.Name)
		b.WriteByte(seps[0])
		b.WriteString(l.Value)
	}
	return b.Bytes()
}

// MarshalJSON implements json.Marshaler.
func (ls Labels) MarshalJSON() ([]byte, error) {
	return json.Marshal(ls.Map())
}

// UnmarshalJSON implements json.Unmarshaler.
func (ls *Labels) UnmarshalJSON(b []byte) error {
	var m map[string]string

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	*ls = FromMap(m)
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (ls Labels) MarshalYAML() (interface{}, error) {
	return ls.Map(), nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (ls *Labels) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var m map[string]string

	if err := unmarshal(&m); err != nil {
		return err
	}

	*ls = FromMap(m)
	return nil
}

// MatchLabels returns a subset of Labels that matches/does not match with the provided label names based on the 'on' boolean.
// If on is set to true, it returns the subset of labels that match with the provided label names and its inverse when 'on' is set to false.
func (ls Labels) MatchLabels(on bool, names ...string) Labels {
	matchedLabels := Labels{}

	nameSet := map[string]struct{}{}
	for _, n := range names {
		nameSet[n] = struct{}{}
	}

	for _, v := range ls {
		if _, ok := nameSet[v.Name]; on == ok && (on || !isInnerLabel(v)) {
			matchedLabels = append(matchedLabels, v)
		}
	}

	return matchedLabels
}

// Hash returns a hash value for the label set.
func (ls Labels) Hash() uint64 {
	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024)
	for i, v := range ls {
		if len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) {
			// If labels entry is 1KB+ do not allocate whole entry.
			h := xxhash.New()
			_, _ = h.Write(b)
			for _, v := range ls[i:] {
				_, _ = h.WriteString(v.Name)
				_, _ = h.Write(seps)
				_, _ = h.WriteString(v.Value)
				_, _ = h.Write(seps)
			}
			return h.Sum64()
		}

		b = append(b, v.Name...)
		b = append(b, seps[0])
		b = append(b, v.Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b)
}

// HashForLabels returns a hash value for the labels matching the provided names.
// 'names' have to be sorted in ascending order.
func (ls Labels) HashForLabels(b []byte, names ...string) (uint64, []byte) {
	b = b[:0]
	i, j := 0, 0
	for i < len(ls) && j < len(names) {
		if names[j] < ls[i].Name {
			j++
		} else if ls[i].Name < names[j] {
			i++
		} else {
			b = append(b, ls[i].Name...)
			b = append(b, seps[0])
			b = append(b, ls[i].Value...)
			b = append(b, seps[0])
			i++
			j++
		}
	}
	return xxhash.Sum64(b), b
}

// HashWithoutLabels returns a hash value for all labels except those matching
// the provided names.
// 'names' have to be sorted in ascending order.
func (ls Labels) HashWithoutLabels(b []byte, names ...string) (uint64, []byte) {
	b = b[:0]
	j := 0
	for i := range ls {
		for j < len(names) && names[j] < ls[i].Name {
			j++
		}
		if isInnerLabel(ls[i]) || (j < len(names) && ls[i].Name == names[j]) {
			continue
		}
		b = append(b, ls[i].Name...)
		b = append(b, seps[0])
		b = append(b, ls[i].Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b), b
}

// WithLabels returns a new labels.Labels from ls that only contains labels matching names.
// 'names' have to be sorted in ascending order.
func (ls Labels) WithLabels(names ...string) Labels {
	ret := make([]Label, 0, len(ls))

	i, j := 0, 0
	for i < len(ls) && j < len(names) {
		if names[j] < ls[i].Name {
			j++
		} else if ls[i].Name < names[j] {
			i++
		} else {
			ret = append(ret, ls[i])
			i++
			j++
		}
	}
	return ret
}

// WithoutLabels returns a new labels.Labels from ls that contains labels not matching names.
// 'names' have to be sorted in ascending order.
func (ls Labels) WithoutLabels(names ...string) Labels {
	ret := make([]Label, 0, len(ls))

	j := 0
	for i := range ls {
		for j < len(names) && names[j] < ls[i].Name {
			j++
		}
		if isInnerLabel(ls[i]) || (j < len(names) && ls[i].Name == names[j]) {
			continue
		}
		ret = append(ret, ls[i])
	}
	return ret
}

// Copy returns a copy of the labels.
func (ls Labels) Copy() Labels {
	res := make(Labels, len(ls))
	copy(res, ls)
	return res
}

// Get returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (ls Labels) Get(name string) string {
	for _, l := range ls {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}

// Has returns true if the label with the given name is present.
func (ls Labels) Has(name string) bool {
	for _, l := range ls {
		if l.Name == name {
			return true
		}
	}
	return false
}

// HasDuplicateLabelNames returns whether ls has duplicate label names.
// It assumes that the labelset is sorted.
func (ls Labels) HasDuplicateLabelNames() (string, bool) {
	for i, l := range ls {
		if i == 0 {
			continue
		}
		if l.Name == ls[i-1].Name {
			return l.Name, true
		}
	}
	return "", false
}

// WithoutEmpty returns the labelset without empty labels.
// May return the same labelset.
func (ls Labels) WithoutEmpty() Labels {
	for _, v := range ls {
		if v.Value != "" {
			continue
		}
		// Do not copy the slice until it's necessary.
		els := make(Labels, 0, len(ls)-1)
		for _, v := range ls {
			if v.Value != "" {
				els = append(els, v)
			}
		}
		return els
	}
	return ls
}

// Map returns a string map of the labels.
func (ls Labels) Map() map[string]string {
	m := make(map[string]string, len(ls))
	for _, l := range ls {
		m[l.Name] = l.Value
	}
	return m
}

// New returns a sorted Labels from the given labels.
// The caller has to guarantee that all label names are unique.
func New(ls ...Label) Labels {
	set := make(Labels, 0, len(ls))
	for _, l := range ls {
		set = append(set, l)
	}
	sort.Sort(set)

	return set
}

// FromMap returns new sorted Labels from the given map.
func FromMap(m map[string]string) Labels {
	l := make([]Label, 0, len(m))
	for k, v := range m {
		l = append(l, Label{Name: k, Value: v})
	}
	return New(l...)
}

func (ls Labels) Keys() []string {
	var keys []string
	for _, l := range ls {
		keys = append(keys, l.Name)
	}
	return keys
}

func (ls *Labels) Append(name, val string) {
	for idx, l := range *ls {
		if l.Name == name {
			(*ls)[idx].Value = val
			return
		}
	}
	*ls = append(*ls, Label{Name: name, Value: val})
}

type Labels []Label
