// 仪表盘页面配置
return {
  children: [
    {
      content: {
        title: '使用统计',
        icon: '/emoji/chart.svg',
        template: 'statistic',
        fields: [
          { label: '在线设备数量', key: 'online', action: { type: 'link', link: '/page/device' } },
          { label: '离线设备数量', key: 'offline', action: { type: 'link', link: '/page/device' } },
          { label: '异常设备数量', key: 'error', action: { type: 'link', link: '/page/device' } },
          { label: '产品数量', key: 'product', action: { type: 'link', link: '/page/product' } },
          { label: '用户数量', key: 'user', action: { type: 'link', link: '/page/user' } },
          { label: '组织数量', key: 'group', action: { type: 'link', link: '/page/group' } }
        ],
        // 页面挂载时执行
        mount() {
          this.data = {}
          this.request.post('table/device/count', { filter: { online: 1 } }).subscribe(res => {
            this.data.online = res.data
          })
          this.request.post('table/device/count', { filter: { online: 0 } }).subscribe(res => {
            this.data.offline = res.data
          })
          this.request.post('table/device/count', { filter: { error: 1 } }).subscribe(res => {
            this.data.error = res.data
          })
          this.request.post('table/product/count', {}).subscribe(res => {
            this.data.product = res.data
          })
          this.request.post('table/user/count', {}).subscribe(res => {
            this.data.user = res.data
          })
          this.request.post('table/group/count', {}).subscribe(res => {
            this.data.group = res.data
          })
        }
      }
    }
  ]
}
