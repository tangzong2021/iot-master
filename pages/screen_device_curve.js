// 大屏趋势曲线: 东山溶解氧浮标 溶解氧近2小时走势, 60秒自动刷新
var CURVE_DEVICE = '864710078897633'
var CURVE_KEY = 'do1_conc'
var CURVE_LABEL = '溶解氧(mg/L)'

return {
  template: 'blank',
  children: [
    {
      span: 24,
      content: {
        title: CURVE_LABEL + ' · 近2小时',
        icon: '/emoji/chart.svg',
        template: 'chart',
        type: 'line',
        time: true,
        theme: 'dark',
        height: 200,
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
              .get('device/' + CURVE_DEVICE + '/history/' + CURVE_KEY, {
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
