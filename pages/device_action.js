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
  },
  submit_api: 'device/:id/action/:action'
}
