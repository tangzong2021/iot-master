// 地图大屏页面配置
return {
  template: 'amap',
  height: '100vh',
  full: true,
  satellite: false,
  mapStyle: 'amap://styles/darkblue',
  zoom: 5,
  ready() {
    this.load_data()
  },
  methods: {
    load_devices() {
      this.request
        .post('table/device/search', {
          limit: 999999,
          fields: ['id', 'longitude', 'latitude', 'online', 'name']
        })
        .subscribe(res => {
          this.render_devices(res.data)
        })
    },
    render_devices(devices) {
      this.addClusters(devices)
    }
  },
  overlay: {
    content: {
      template: 'blank',
      children: [
        {
          page: 'screen_title'
        },
        {
          span: 6,
          content: {
            template: 'blank',
            children: [
              {
                page: 'screen_product_chart'
              }
            ]
          }
        },
        {
          span: 12,
          content: {
            template: 'statistic',
            style: { margin: '5px' },
            style2: { color: 'white', background: 'transparent' },
            bodyStyle: { color: 'white', background: 'transparent' },
            fields: [
              { label: '总数', key: 'total' },
              { label: '在线', key: 'online' },
              { label: '故障', key: 'error' }
            ],
            mount() {
              this.data = {}
              this.request.post('table/device/count', {}).subscribe(res => {
                this.data.total = res.data
              })
              this.request.post('table/device/count', { filter: { online: 1 } }).subscribe(res => {
                this.data.online = res.data
              })
              this.request.post('table/device/count', { filter: { error: 1 } }).subscribe(res => {
                this.data.error = res.data
              })
            }
          }
        },
        {
          span: 6,
          content: {
            template: 'blank',
            children: [
              {
                content: {
                  title: '各产品设备数量',
                  icon: '/emoji/chart.svg',
                  template: 'chart',
                  style: { margin: '5px' },
                  type: 'bar',
                  theme: 'dark',
                  bodyStyle: { color: 'white', padding: 0 },
                  mount() {
                    this.request.post('table/device/group', {
                      by: ['product_id'],
                      aggregators: [{ func: 'count', field: 'id', as: 'cnt' }],
                      joins: [
                        { table: 'product', local: 'product_id', foreign: 'id', fields: { name: 'product_name' } }
                      ]
                    }).subscribe(res => {
                      const data = [['产品', '设备数'], ...(res.data || []).map(i => [i.product_name || '未知产品', i.cnt])]
                      this.render(data)
                    })
                  }
                }
              },
              {
                content: {
                  title: '报警日志',
                  template: 'list',
                  style: { margin: '5px' },
                  bodyStyle: {
                    color: 'white',
                    'background-color': 'black',
                    padding: 0
                  }
                }
              }
            ]
          }
        }
      ]
    }
  }
}
