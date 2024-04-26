package application

import (
	"context"
	"mayfly-go/internal/tag/domain/entity"
	"mayfly-go/internal/tag/domain/repository"
	"mayfly-go/pkg/base"
	"mayfly-go/pkg/contextx"
	"mayfly-go/pkg/errorx"
	"mayfly-go/pkg/utils/collx"
	"time"
)

type ResourceOpLog interface {
	base.App[*entity.ResourceOpLog]

	// AddResourceOpLog 新增资源操作记录
	AddResourceOpLog(ctx context.Context, codePath string) error
}

type resourceOpLogAppImpl struct {
	base.AppImpl[*entity.ResourceOpLog, repository.ResourceOpLog]

	tagTreeApp TagTree `inject:"TagTreeApp"`
}

var _ (ResourceOpLog) = (*resourceOpLogAppImpl)(nil)

// 注入ResourceOpLogRepo
func (rol *resourceOpLogAppImpl) InjectResourceOpLogRepo(resourceOpLogRepo repository.ResourceOpLog) {
	rol.Repo = resourceOpLogRepo
}

func (rol *resourceOpLogAppImpl) AddResourceOpLog(ctx context.Context, codePath string) error {
	loginAccount := contextx.GetLoginAccount(ctx)
	if loginAccount == nil {
		return errorx.NewBiz("当前上下文不存在登录信息")
	}

	var logs []*entity.ResourceOpLog
	if err := rol.ListByWheres(collx.Kvs("create_time > ?", time.Now().Add(-5*time.Minute), "creator_id = ?", loginAccount.Id, "code_path = ?", codePath), &logs); err != nil {
		return err
	}
	// 指定时间内多次操作则不记录
	if len(logs) > 0 {
		return nil
	}
	tagTree := &entity.TagTree{CodePath: codePath}
	if err := rol.tagTreeApp.GetBy(tagTree); err != nil {
		return errorx.NewBiz("资源不存在")
	}

	return rol.Save(ctx, &entity.ResourceOpLog{
		ResourceCode: tagTree.Code,
		ResourceType: int8(tagTree.Type),
		CodePath:     tagTree.CodePath,
	})
}
