// 升级记录页面配置：展示固件升级任务下发与设备回报结果
return {
  title: '升级记录',
  icon: '/emoji/version.svg',
  template: 'list',
  toolbar: [
    {
      type: 'button',
      icon: 'reload',
      label: '刷新',
      action: {
        type: 'script',
        script(data, index) {
          this.load()
        }
      }
    },
    { key: 'keyword', type: 'text', placeholder: '请输入关键字' },
    {
      key: 'status',
      type: 'select',
      placeholder: '状态',
      clear: true,
      options: [
        { label: '已下发', value: '已下发' },
        { label: '下载中', value: '下载中' },
        { label: '成功', value: '成功' },
        { label: '失败', value: '失败' }
      ]
    },
    {
      type: 'button',
      icon: 'search',
      label: '搜索',
      action: {
        type: 'script',
        script(data, index) {
          const v = this.toolbar.value || {}
          this.keyword = v.keyword || ''
          this.filter.status = v.status || undefined
          this.load()
        }
      }
    }
  ],
  keywords: ['id', 'device_id', 'msg_id'],
  operators: [
    {
      icon: 'delete',
      title: '删除',
      confirm: '确认删除？',
      action: {
        type: 'script',
        script(data, index) {
          this.request.get('table/upgrade/delete/' + data.id).subscribe(res => {
            this.load()
          })
        }
      }
    }
  ],
  batch: true,
  fields: [
    { key: 'id', label: 'ID', type: 'text' },
    {
      key: 'device_name',
      label: '设备',
      type: 'text',
      action: {
        type: 'page',
        page: 'device_detail',
        params(data) {
          return { id: data.device_id }
        }
      }
    },
    { key: 'device_id', label: '设备ID', type: 'text' },
    { key: 'version_name', label: '目标版本', type: 'text' },
    { key: 'from_version', label: '原版本', type: 'text' },
    { key: 'status', label: '状态', type: 'text' },
    { key: 'error', label: '错误', type: 'text' },
    { key: 'created', label: '下发时间', type: 'datetime', sortable: true, sort: -1 },
    { key: 'updated', label: '更新时间', type: 'datetime' }
  ],
  search_api: 'table/upgrade/search'
}
