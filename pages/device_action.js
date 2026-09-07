// 执行动作页面配置
return {
  title: '执行动作',
  icon: '/emoji/device.svg',
  template: 'edit',
  close_on_error: true, //下发超时等错误时提示并自动关闭弹窗
  fields: [],
  // 页面挂载时执行
  mount() {
    if (this.params.title) this.content.title = this.params.title
    this.content.fields = this.params.parameters || []
    this.content.fields.forEach(f => {
      if (f.data_api) {
        this.request.get(f.data_api).subscribe(res => {
          if (res.error) return
          f.options = res.data.data || res.data
        })
      }
    })
    this.bind_current_state()
  },
  submit_api: 'device/:id/action/:action',
  methods: {
    // 数据绑定(bind=设备数值键名): 打开弹窗时从设备最新数值回显当前状态到表单默认值
    // 设备每次上报都携带状态键(如 interval_min), 动作参数声明 bind 即可自动回显
    bind_current_state() {
      const binds = (this.content.fields || []).filter(f => f.bind)
      if (!binds.length) return
      this.request.get('device/' + this.params.id + '/values').subscribe(res => {
        if (res.error || !res.data) return
        binds.forEach(f => {
          const cur = res.data[f.bind]
          if (cur === undefined || cur === null) return
          if (f.type === 'switch') f.default = (cur === true || cur === 1 || cur === '1' || cur === 'true')
          else f.default = cur
        })
        const desc = binds.map(f => {
          const v = res.data[f.bind]
          return (f.label || f.key) + ': ' + (v === undefined || v === null ? '-' : v) + (f.unit || '')
        }).join('，')
        this.notification.info('当前状态', desc)
        setTimeout(() => { if (this.editor) this.editor.rebuild() }, 100)
      })
    }
  }
}
