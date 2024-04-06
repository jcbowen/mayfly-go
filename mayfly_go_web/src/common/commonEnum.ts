import EnumValue from './Enum';

// 标签关联的资源类型
export const TagResourceTypeEnum = {
    Tag: EnumValue.of(-1, '标签').setExtra({ icon: 'CollectionTag' }),
    Machine: EnumValue.of(1, '机器').setExtra({ icon: 'Monitor' }),
    Db: EnumValue.of(2, '数据库').setExtra({ icon: 'Coin' }),
    Redis: EnumValue.of(3, 'redis').setExtra({ icon: 'iconfont icon-redis' }),
    Mongo: EnumValue.of(4, 'mongo').setExtra({ icon: 'iconfont icon-mongo' }),
};
