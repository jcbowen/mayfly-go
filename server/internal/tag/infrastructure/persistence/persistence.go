package persistence

import (
	"mayfly-go/pkg/ioc"
)

func InitIoc() {
	ioc.Register(newTagTreeRepo(), ioc.WithComponentName("TagTreeRepo"))
	ioc.Register(newTagTreeTeamRepo(), ioc.WithComponentName("TagTreeTeamRepo"))
	ioc.Register(newTagResourceRepo(), ioc.WithComponentName("TagResourceRepo"))
	ioc.Register(newTeamRepo(), ioc.WithComponentName("TeamRepo"))
	ioc.Register(newTeamMemberRepo(), ioc.WithComponentName("TeamMemberRepo"))
}
