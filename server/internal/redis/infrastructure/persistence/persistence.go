package persistence

import (
	"mayfly-go/pkg/ioc"
)

func InitIoc() {
	ioc.Register(newRedisRepo(), ioc.WithComponentName("RedisRepo"))
}
