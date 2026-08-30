// 历史曲线页面配置
return {
  title: '历史曲线',
  template: 'chart',
  type: 'line',
  toolbar: [
    {
      key: 'start',
      type: 'datetime',
      label: '开始时间'
    },
    {
      key: 'end',
      type: 'datetime',
      label: '结束时间'
    },
    {
      key: 'window',
      type: 'number',
      default: '5',
      label: '窗口'
    },
    {
      key: 'unit',
      type: 'select',
      default: 'm',
      options: [
        { value: 's', label: '秒' },
        { value: 'm', label: '分钟' },
        { value: 'h', label: '小时' },
        { value: 'd', label: '天' }
      ]
    },
    {
      key: 'method',
      type: 'select',
      label: '算子',
      default: 'last',
      options: [
        { value: 'last', label: '最后值' },
        { value: 'mean', label: '均值' }
      ]
    },
    {
      type: 'button',
      label: '查询',
      action: {
        type: 'script',
        script(data, index) {
          this.load_history()
        }
      }
    },
    {
      type: 'button',
      label: '导出CSV',
      action: {
        type: 'script',
        script(data, index) {
          //导出当前时间范围内该设备的【全部因子】，按时间对齐合并为一个多列CSV
          const doExport = (pid, points) => {
            if (!points || !points.length) {
              alert('产品未定义物模型点位，无法导出')
              return
            }
            const query = {
              start: this.dayjs(this.toolbar.value.start).toISOString(),
              end: this.dayjs(this.toolbar.value.end).toISOString(),
              window: this.toolbar.value.window + this.toolbar.value.unit,
              method: this.toolbar.value.method
            }
            //逐点位拉取历史数据
            Promise.all(points.map(p => {
              return new Promise(resolve => {
                this.request.get('device/' + this.params.id + '/history/' + p.name, query)
                  .subscribe(res => resolve(res.data || []))
              })
            })).then(list => {
              //按时间戳合并多因子行
              const table = {}
              list.forEach((records, idx) => {
                const name = points[idx].name
                records.map(r => {
                  const row = table[r.time] || (table[r.time] = {})
                  row[name] = r.value
                })
              })
              const times = Object.keys(table).map(Number).sort((a, b) => a - b)
              if (!times.length) {
                alert('当前时间范围内没有数据')
                return
              }
              const rows = ['时间,' + points.map(p => p.label + '(' + p.name + ')').join(',')]
              times.map(t => {
                rows.push(this.dayjs(t).format('YYYY-MM-DD HH:mm:ss') + ',' + points.map(p => {
                  const v = table[t][p.name]
                  return v === undefined ? '' : v
                }).join(','))
              })
              //\ufeff BOM头，保证Excel打开中文不乱码
              const blob = new Blob(['\ufeff' + rows.join('\n')], {type: 'text/csv;charset=utf-8'})
              const url = URL.createObjectURL(blob)
              const a = document.createElement('a')
              a.href = url
              a.download = this.params.id + '-all-points' + this.dayjs().format('-YYYYMMDDHHmmss') + '.csv'
              document.body.appendChild(a)
              a.click()
              document.body.removeChild(a)
              URL.revokeObjectURL(url)
            })
          }
          //优先用页面参数里的产品ID，没有就查设备详情
          if (this.params.product_id) {
            const pid = this.params.product_id
            this.request.get('product/' + pid + '/setting/model').subscribe(res => {
              const points = []
              ;(res.data && res.data.content ? res.data.content : []).map(p => (p.points || []).map(pt => points.push(pt)))
              doExport(pid, points)
            })
          } else {
            this.request.get('table/device/detail/' + this.params.id).subscribe(res => {
              if (res.error || !res.data || !res.data.product_id) {
                alert('找不到设备所属产品')
                return
              }
              const pid = res.data.product_id
              this.request.get('product/' + pid + '/setting/model').subscribe(r2 => {
                const points = []
                ;(r2.data && r2.data.content ? r2.data.content : []).map(p => (p.points || []).map(pt => points.push(pt)))
                doExport(pid, points)
              })
            })
          }
        }
      }
    },
    {
      type: 'link',
      label: '过去1天',
      action: {
        type: 'script',
        script(data, index) {
          this.toolbarValue = {
            start: this.dayjs().subtract(1, 'day').format('YYYY-MM-DD HH:mm:ss'),
            end: this.dayjs().format('YYYY-MM-DD HH:mm:ss'),
            window: 5,
            unit: 'm'
          }
          setTimeout(() => this.load_history(), 100)
        }
      }
    },
    {
      type: 'link',
      label: '过去1小时',
      action: {
        type: 'script',
        script(data, index) {
          this.toolbarValue = {
            start: this.dayjs().subtract(1, 'hour').format('YYYY-MM-DD HH:mm:ss'),
            end: this.dayjs().format('YYYY-MM-DD HH:mm:ss'),
            window: 10,
            unit: 's'
          }
          setTimeout(() => this.load_history(), 100)
        }
      }
    },
    {
      type: 'link',
      label: '过去10分钟',
      action: {
        type: 'script',
        script(data, index) {
          this.toolbarValue = {
            start: this.dayjs().subtract(10, 'minute').format('YYYY-MM-DD HH:mm:ss'),
            end: this.dayjs().format('YYYY-MM-DD HH:mm:ss'),
            window: 1,
            unit: 's'
          }
          setTimeout(() => this.load_history(), 100)
        }
      }
    }
  ],
  time: true,
  options: {
    tooltip: {
      trigger: 'axis'
    }
  },
  // 页面挂载时执行
  mount() {
    this.toolbarValue = {
      start: this.dayjs().subtract(1, 'day').format('YYYY-MM-DD HH:mm:ss')
    }
    setTimeout(() => this.load_history(), 100)
  },
  methods: {
    load_history() {
      this.request
        .get('device/' + this.params.id + '/history/' + this.params.point, {
          start: this.dayjs(this.toolbar.value.start).toISOString(),
          end: this.dayjs(this.toolbar.value.end).toISOString(),
          window: this.toolbar.value.window + this.toolbar.value.unit,
          method: this.toolbar.value.method
        })
        .subscribe(res => {
          if (res.data) {
            this.render(
              res.data.map(i => {
                return [i.time, i.value]
              })
            )
          }
        })
    }
  }
}
