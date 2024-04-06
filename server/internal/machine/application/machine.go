package application

import (
	"context"
	"fmt"
	"mayfly-go/internal/common/consts"
	"mayfly-go/internal/machine/api/vo"
	"mayfly-go/internal/machine/domain/entity"
	"mayfly-go/internal/machine/domain/repository"
	"mayfly-go/internal/machine/infrastructure/cache"
	"mayfly-go/internal/machine/mcm"
	tagapp "mayfly-go/internal/tag/application"
	"mayfly-go/pkg/base"
	"mayfly-go/pkg/errorx"
	"mayfly-go/pkg/global"
	"mayfly-go/pkg/logx"
	"mayfly-go/pkg/model"
	"mayfly-go/pkg/scheduler"
	"time"
)

type Machine interface {
	base.App[*entity.Machine]

	SaveMachine(ctx context.Context, m *entity.Machine, tagIds ...uint64) error

	// 测试机器连接
	TestConn(me *entity.Machine) error

	// 调整机器状态
	ChangeStatus(ctx context.Context, id uint64, status int8) error

	Delete(ctx context.Context, id uint64) error

	// 分页获取机器信息列表
	GetMachineList(condition *entity.MachineQuery, pageParam *model.PageParam, toEntity *[]*vo.MachineVO, orderBy ...string) (*model.PageResult[*[]*vo.MachineVO], error)

	// 新建机器客户端连接（需手动调用Close）
	NewCli(id uint64) (*mcm.Cli, error)

	// 获取已缓存的机器连接，若不存在则新建客户端连接并缓存，主要用于定时获取状态等（避免频繁创建连接）
	GetCli(id uint64) (*mcm.Cli, error)

	// 获取ssh隧道机器连接
	GetSshTunnelMachine(id int) (*mcm.SshTunnelMachine, error)

	// 定时更新机器状态信息
	TimerUpdateStats()

	// 获取机器运行时状态信息
	GetMachineStats(machineId uint64) (*mcm.Stats, error)

	ToMachineInfoById(machineId uint64) (*mcm.MachineInfo, error)
}

type machineAppImpl struct {
	base.AppImpl[*entity.Machine, repository.Machine]

	authCertApp AuthCert       `inject:"AuthCertApp"`
	tagApp      tagapp.TagTree `inject:"TagTreeApp"`
}

// 注入MachineRepo
func (m *machineAppImpl) InjectMachineRepo(repo repository.Machine) {
	m.Repo = repo
}

// 分页获取机器信息列表
func (m *machineAppImpl) GetMachineList(condition *entity.MachineQuery, pageParam *model.PageParam, toEntity *[]*vo.MachineVO, orderBy ...string) (*model.PageResult[*[]*vo.MachineVO], error) {
	return m.GetRepo().GetMachineList(condition, pageParam, toEntity, orderBy...)
}

func (m *machineAppImpl) SaveMachine(ctx context.Context, me *entity.Machine, tagIds ...uint64) error {
	oldMachine := &entity.Machine{
		Ip:                 me.Ip,
		Port:               me.Port,
		Username:           me.Username,
		SshTunnelMachineId: me.SshTunnelMachineId,
	}

	err := m.GetBy(oldMachine)

	if errEnc := me.PwdEncrypt(); errEnc != nil {
		return errorx.NewBiz(errEnc.Error())
	}
	if me.Id == 0 {
		if err == nil {
			return errorx.NewBiz("该机器信息已存在")
		}
		if m.CountByCond(&entity.Machine{Code: me.Code}) > 0 {
			return errorx.NewBiz("该编码已存在")
		}

		// 新增机器，默认启用状态
		me.Status = entity.MachineStatusEnable

		return m.Tx(ctx, func(ctx context.Context) error {
			return m.Insert(ctx, me)
		}, func(ctx context.Context) error {
			return m.tagApp.SaveResource(ctx, &tagapp.SaveResourceTagParam{
				ResourceCode: me.Code,
				ResourceType: consts.TagResourceTypeMachine,
				TagIds:       tagIds,
			})
		})
	}

	// 如果存在该库，则校验修改的库是否为该库
	if err == nil && oldMachine.Id != me.Id {
		return errorx.NewBiz("该机器信息已存在")
	}
	// 如果调整了ssh username等会查不到旧数据，故需要根据id获取旧信息将code赋值给标签进行关联
	if oldMachine.Code == "" {
		oldMachine, _ = m.GetById(new(entity.Machine), me.Id)
	}

	// 关闭连接
	mcm.DeleteCli(me.Id)
	// 防止误传修改
	me.Code = ""
	return m.Tx(ctx, func(ctx context.Context) error {
		return m.UpdateById(ctx, me)
	}, func(ctx context.Context) error {
		return m.tagApp.SaveResource(ctx, &tagapp.SaveResourceTagParam{
			ResourceCode: oldMachine.Code,
			ResourceType: consts.TagResourceTypeMachine,
			TagIds:       tagIds,
		})
	})
}

func (m *machineAppImpl) TestConn(me *entity.Machine) error {
	me.Id = 0
	mi, err := m.toMachineInfo(me)
	if err != nil {
		return err
	}
	cli, err := mi.Conn()
	if err != nil {
		return err
	}
	cli.Close()
	return nil
}

func (m *machineAppImpl) ChangeStatus(ctx context.Context, id uint64, status int8) error {
	if status == entity.MachineStatusDisable {
		// 关闭连接
		mcm.DeleteCli(id)
	}
	machine := new(entity.Machine)
	machine.Id = id
	machine.Status = status
	return m.UpdateById(ctx, machine)
}

// 根据条件获取机器信息
func (m *machineAppImpl) Delete(ctx context.Context, id uint64) error {
	machine, err := m.GetById(new(entity.Machine), id)
	if err != nil {
		return errorx.NewBiz("机器信息不存在")
	}
	// 关闭连接
	mcm.DeleteCli(id)

	// 发布机器删除事件
	global.EventBus.Publish(ctx, consts.DeleteMachineEventTopic, machine)
	return m.Tx(ctx,
		func(ctx context.Context) error {
			return m.DeleteById(ctx, id)
		}, func(ctx context.Context) error {
			return m.tagApp.SaveResource(ctx, &tagapp.SaveResourceTagParam{
				ResourceCode: machine.Code,
				ResourceType: consts.TagResourceTypeMachine,
			})
		})
}

func (m *machineAppImpl) NewCli(machineId uint64) (*mcm.Cli, error) {
	if mi, err := m.ToMachineInfoById(machineId); err != nil {
		return nil, err
	} else {
		return mi.Conn()
	}
}

func (m *machineAppImpl) GetCli(machineId uint64) (*mcm.Cli, error) {
	return mcm.GetMachineCli(machineId, func(mid uint64) (*mcm.MachineInfo, error) {
		return m.ToMachineInfoById(mid)
	})
}

func (m *machineAppImpl) GetSshTunnelMachine(machineId int) (*mcm.SshTunnelMachine, error) {
	return mcm.GetSshTunnelMachine(machineId, func(mid uint64) (*mcm.MachineInfo, error) {
		return m.ToMachineInfoById(mid)
	})
}

func (m *machineAppImpl) TimerUpdateStats() {
	logx.Debug("开始定时收集并缓存服务器状态信息...")
	scheduler.AddFun("@every 2m", func() {
		machineIds := new([]entity.Machine)
		m.GetRepo().ListByCond(&entity.Machine{Status: entity.MachineStatusEnable, Protocol: entity.MachineProtocolSsh}, machineIds, "id")
		for _, ma := range *machineIds {
			go func(mid uint64) {
				defer func() {
					if err := recover(); err != nil {
						logx.ErrorTrace(fmt.Sprintf("定时获取机器[id=%d]状态信息失败", mid), err.(error))
					}
				}()
				logx.Debugf("定时获取机器[id=%d]状态信息开始", mid)
				cli, err := m.GetCli(mid)
				// ssh获取客户端失败，则更新机器状态为禁用
				if err != nil {
					updateMachine := &entity.Machine{Status: entity.MachineStatusDisable}
					updateMachine.Id = mid
					now := time.Now()
					updateMachine.UpdateTime = &now
					m.UpdateById(context.TODO(), updateMachine)
				}
				cache.SaveMachineStats(mid, cli.GetAllStats())
				logx.Debugf("定时获取机器[id=%d]状态信息结束", mid)
			}(ma.Id)
		}
	})
}

func (m *machineAppImpl) GetMachineStats(machineId uint64) (*mcm.Stats, error) {
	return cache.GetMachineStats(machineId)
}

// 生成机器信息，根据授权凭证id填充用户密码等
func (m *machineAppImpl) ToMachineInfoById(machineId uint64) (*mcm.MachineInfo, error) {
	me, err := m.GetById(new(entity.Machine), machineId)
	if err != nil {
		return nil, errorx.NewBiz("机器信息不存在")
	}
	if me.Status != entity.MachineStatusEnable && me.Protocol == 1 {
		return nil, errorx.NewBiz("该机器已被停用")
	}

	if mi, err := m.toMachineInfo(me); err != nil {
		return nil, err
	} else {
		return mi, nil
	}
}

func (m *machineAppImpl) toMachineInfo(me *entity.Machine) (*mcm.MachineInfo, error) {
	mi := new(mcm.MachineInfo)
	mi.Id = me.Id
	mi.Name = me.Name
	mi.Ip = me.Ip
	mi.Port = me.Port
	mi.Username = me.Username
	mi.TagPath = m.tagApp.ListTagPathByResource(consts.TagResourceTypeMachine, me.Code)
	mi.EnableRecorder = me.EnableRecorder
	mi.Protocol = me.Protocol

	if me.UseAuthCert() {
		ac, err := m.authCertApp.GetById(new(entity.AuthCert), uint64(me.AuthCertId))
		if err != nil {
			return nil, errorx.NewBiz("授权凭证信息已不存在，请重新关联")
		}
		mi.AuthMethod = ac.AuthMethod
		if err := ac.PwdDecrypt(); err != nil {
			return nil, errorx.NewBiz(err.Error())
		}
		mi.Password = ac.Password
		mi.Passphrase = ac.Passphrase
	} else {
		mi.AuthMethod = entity.AuthCertAuthMethodPassword
		if me.Id != 0 {
			if err := me.PwdDecrypt(); err != nil {
				return nil, errorx.NewBiz(err.Error())
			}
		}
		mi.Password = me.Password
	}

	// 使用了ssh隧道，则将隧道机器信息也附上
	if me.SshTunnelMachineId > 0 {
		sshTunnelMe, err := m.GetById(new(entity.Machine), uint64(me.SshTunnelMachineId))
		if err != nil {
			return nil, errorx.NewBiz("隧道机器信息不存在")
		}
		sshTunnelMi, err := m.toMachineInfo(sshTunnelMe)
		if err != nil {
			return nil, err
		}
		mi.SshTunnelMachine = sshTunnelMi
	}
	return mi, nil
}
