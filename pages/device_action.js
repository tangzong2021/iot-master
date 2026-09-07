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
  // 设备回执成功后, 把确认过的周期写入平台设备设置, 下次弹窗即时回显(不等下一条数据)
  submit_success(data) {
    const n = data && (data.interval !== undefined ? data.interval : (data.enable === true ? 5 : (data.enable === false ? 30 : undefined)))
    if (n === 5 || n === 30) {
      this.request.post('device/' + this.params.id + '/setting/interval_state', { interval: n }).subscribe()
    }
  },
  methods: {
    // 数据绑定(bind=设备数值键名): 打开弹窗回显当前状态
    // 优先读平台记忆(设备回执确认过的状态, 即时), 设备最新数值兜底(设备重启恢复30分钟后以设备为准)
    bind_current_state() {
      const binds = (this.content.fields || []).filter(f => f.bind)
      if (!binds.length) return
      const apply = (vals) => {
        binds.forEach(f => {
          const cur = vals[f.bind]
          if (cur === undefined || cur === null) return
          if (f.type === 'switch') f.default = (cur === true || cur === 1 || cur === '1' || cur === 'true')
          else f.default = cur
        })
        const desc = binds.map(f => {
          const v = vals[f.bind]
          return (f.label || f.key) + ': ' + (v === undefined || v === null ? '-' : v) + (f.unit || '')
        }).join('，')
        this.notification.info('当前状态', desc)
        setTimeout(() => { if (this.editor) this.editor.rebuild() }, 100)
      }
      this.request.get('device/' + this.params.id + '/setting/interval_state').subscribe(res => {
        if (!res.error && res.data && res.data[binds[0].bind] !== undefined) {
          apply(res.data)
          return
        }
        this.request.get('device/' + this.params.id + '/values').subscribe(res2 => {
          if (res2.error || !res2.data) return
          apply(res2.data)
        })
      })
    }
  }
}
