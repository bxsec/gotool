package pool

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPool(t *testing.T) {
	// not pool anything yet
	UsePool = false
	// THE Magic Number
	magicNumber := 42

	intType := reflect.TypeOf(magicNumber)
	// init int pool
	ReflectTypePools.Init(intType)
	// insert a integer
	ReflectTypePools.Put(intType, magicNumber)
	// if UsePool == false, argsReplyPools.Get() will call reflect.New() which
	// returns a Value representing a pointer to a new zero value
	assert.Equal(t, 0, *ReflectTypePools.Get(intType).(*int))

	// start pooling
	UsePool = true

	ReflectTypePools.Put(intType, magicNumber)
	// Get() will remove element from pool
	assert.Equal(t, magicNumber, ReflectTypePools.Get(intType).(int))
}
