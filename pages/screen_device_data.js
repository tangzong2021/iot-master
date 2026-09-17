// 大屏实时数据卡片: 按物模型精度展示东山溶解氧浮标关键因子, 30秒自动刷新
var DATA_DEVICE = '864710078897633'

return {
  template: 'blank',
  children: [
    {
      span: 24,
      content: {
        title: '实时数据 · 东山溶解氧',
        icon: '/emoji/dashboard.svg',
        template: 'statistic',
        style: { margin: '5px' },
        style2: { color: 'white', background: 'transparent' },
        bodyStyle: { color: 'white', background: 'transparent' },
        fields: [
          { label: '溶解氧', key: 'do1_conc', suffix: 'mg/L' },
          { label: '电压', key: 'volt', suffix: 'V' }
        ],
        mount() {
          var self = this
          self.data = {}
          var fmt = { do1_conc: 2, volt: 1 }
          var load = function () {
            self.request.get('device/' + DATA_DEVICE + '/values').subscribe(function (res) {
              if (res.error || !res.data) return
              var d = res.data
              Object.keys(fmt).forEach(function (k) {
                if (typeof d[k] === 'number') d[k] = d[k].toFixed(fmt[k])
              })
              self.data = d
            })
          }
          load()
          setInterval(load, 30000)
        }
      }
    }
  ]
}
