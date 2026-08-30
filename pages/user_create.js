// 创建用户页面配置
return {
  title: '创建用户',
  icon: '/emoji/user.svg',
  template: 'edit',
  fields: [
    { key: 'id', label: 'ID', type: 'text', required: true },
    { key: 'name', label: '名称', type: 'text', required: true },
    { key: 'avatar', label: '头像', type: 'file', upload: '/api/upload' },
    { key: 'email', label: '邮箱', type: 'text' },
    { key: 'cellphone', label: '手机', type: 'text' },
    { key: 'oem', label: 'OEM', type: 'text' },
    { key: 'admin', label: '管理员', type: 'switch', admin: true },
    { key: 'disabled', label: '禁用', type: 'switch' },
    { key: 'priv_data_view', label: '数据查看', type: 'checkbox' },
    { key: 'priv_device_manage', label: '设备管理', type: 'checkbox' },
    { key: 'priv_system', label: '系统管理', type: 'checkbox' }
  ],
  submit_api: 'table/user/create'
}
