// 编辑用户页面配置
return {
  title: '编辑用户',
  icon: '/emoji/user.svg',
  template: 'edit',
  fields: [
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
  toolbar: [
    { type: 'button', label: '站点授权', action: { type: 'dialog', page: 'site_auth', params(data, index) { return {id: this.params.id} } } }
  ],
  load_api: 'table/user/detail/:id',
  submit_api: 'table/user/update/:id'
}
