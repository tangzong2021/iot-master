// 大屏设备监控面板: 实时数据卡片 + 近2小时趋势曲线
// 监控设备与因子在下方配置, 改动同步 PVC 即时生效
var MONITOR_DEVICE = '864710078897633'
var MONITOR_NAME = '东山溶解氧'
var CURVE_KEY = 'do1_conc'
var CURVE_LABEL = '溶解氧(mg/L)'

return {
  template: 'blank',
  children: [
    {
      span: 24,
      content: {
        title: '实时数据 · ' + MONITOR_NAME,
        icon: '/emoji/dashboard.svg',
        template: 'statistic',
        style: { margin: '5px' },
        style2: { color: 'white', background: 'transparent' },
        bodyStyle: { color: 'white', background: 'transparent' },
        fields: [
          { label: '溶解氧', key: 'do1_conc', suffix: 'mg/L' },
          { label: '水温', key: 'do1_temp', suffix: '℃' },
          { label: '电池电压', key: 'volt', suffix: 'V' },
          { label: '舱内湿度', key: 'cabin_humidity', suffix: '%RH' }
        ],
        mount() {
          var self = this
          self.data = {}
          var fmt = { do1_conc: 2, do1_temp: 2, volt: 1, cabin_humidity: 1 }
          var load = function () {
            self.request.get('device/' + MONITOR_DEVICE + '/values').subscribe(function (res) {
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
    },
    {
      span: 24,
      content: {
        title: CURVE_LABEL + ' · 近2小时',
        icon: '/emoji/chart.svg',
        template: 'chart',
        type: 'line',
        time: true,
        theme: 'dark',
        legend: false,
        tooltip: true,
        style: { margin: '5px' },
        bodyStyle: { color: 'white', padding: 0 },
        mount() {
          this.load_curve()
          setInterval(() => this.load_curve(), 60000)
        },
        methods: {
          load_curve() {
            var self = this
            var end = Date.now()
            var start = end - 2 * 3600 * 1000
            this.request
              .get('device/' + MONITOR_DEVICE + '/history/' + CURVE_KEY, {
                start: new Date(start).toISOString(),
                end: new Date(end).toISOString(),
                window: '5m',
                method: 'last'
              })
              .subscribe(res => {
                var rows = [['时间', CURVE_LABEL]]
                ;(res.data || []).forEach(r => {
                  if (r.value === null || r.value === undefined) return
                  rows.push([r.time, Math.round(r.value * 100) / 100])
                })
                if (rows.length > 1) self.render(rows)
              })
          }
        }
      }
    }
  ]
}
