// 下发升级页面配置：选择设备（可多选），向平台请求下发升级指令
return {
  title: '下发升级',
  icon: '/emoji/version.svg',
  template: 'edit',
  close_on_error: true,
  fields: [
    {
      key: 'device_id',
      label: '选择设备',
      type: 'select',
      multiple: true,
      showSearch: true,
      dropdownStyle: { minWidth: '380px', maxHeight: '380px' },
      placeholder: '选择要升级的设备（可多选）',
      description: '只有在线设备能收到升级指令'
    }
  ],
  // 自定义提交：携带 params.id（固件版本ID）
  submit(data) {
    this.request.post('upgrade/create', {
      device_id: data.device_id,
      version_id: this.params.id
    }).subscribe(res => {
      if (res.error) {
        this.notification.error('提示', res.error)
        return
      }
      this.notification.success('提示', '升级指令已下发，可在 升级记录 中查看进度')
      if (this.modalRef && !this.isChild) this.modalRef.close()
    })
  },
  // 页面挂载时执行：显示目标版本，并加载同产品的设备选项
  mount() {
    this.request.get('table/version/detail/' + this.params.id).subscribe(res => {
      if (!res.error && res.data) this.content.title = '下发升级：' + (res.data.name || res.data.id)
    })
    this.request.post('table/device/search', {
      skip: 0,
      limit: 999,
      filter: this.params.product_id ? { product_id: this.params.product_id } : {}
    }).subscribe(res => {
      if (res.error) return
      const rows = res.data.data || res.data || []
      const f = this.content.fields[0]
      f.options = rows.map(r => ({
        label: (r.name || r.id) + (r.online ? '（在线）' : '（离线）'),
        value: r.id
      }))
    })
  }
}
