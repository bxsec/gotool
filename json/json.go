package json

import (
	"encoding/json"
	"github.com/tidwall/gjson"
)

var (
	// Marshal is exported by component-base/pkg/json package.
	Marshal = json.Marshal
	// Unmarshal is exported by component-base/pkg/json package.
	Unmarshal = json.Unmarshal
	// MarshalIndent is exported by component-base/pkg/json package.
	MarshalIndent = json.MarshalIndent
	// NewDecoder is exported by component-base/pkg/json package.
	NewDecoder = json.NewDecoder
	// NewEncoder is exported by component-base/pkg/json package.
	NewEncoder = json.NewEncoder

	// gjson 封装
	Get = gjson.Get
	AddModifier = gjson.AddModifier
	ForEachLine = gjson.ForEachLine
	Valid = gjson.Valid
	GetMany = gjson.GetMany
)