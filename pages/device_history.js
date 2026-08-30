// 历史曲线页面配置：默认不绘制，下拉框点选因子加入曲线，每因子独立纵轴自适应，图例点选显隐
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
      key: 'factor_sel',
      type: 'select',
      label: '选择因子',
      multiple: true,
      placeholder: '点选要查看的因子（可多选）',
      options: []
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
      label: '清空曲线',
      action: {
        type: 'script',
        script(data, index) {
          try { this.toolbar.group.get('factor_sel').setValue([]) } catch (e) { }
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
          //导出当前已选因子，按时间对齐合并为一个多列CSV
          const selNames = (this.toolbar.value && this.toolbar.value.factor_sel) || []
          const points = (this.points || []).filter(p => selNames.includes(p.name))
          if (!points.length) {
            alert('请先选择因子')
            return
          }
          const query = {
            start: this.dayjs(this.toolbar.value.start).toISOString(),
            end: this.dayjs(this.toolbar.value.end).toISOString(),
            window: this.toolbar.value.window + this.toolbar.value.unit,
            method: this.toolbar.value.method
          }
          Promise.all(points.map(p => {
            return new Promise(resolve => {
              this.request.get('device/' + this.params.id + '/history/' + p.name, query)
                .subscribe(res => resolve(res.data || []))
            })
          })).then(list => {
            const table = {}
            const times = []
            list.forEach((records, idx) => {
              const name = points[idx].name
              records.map(r => {
                if (table[r.time] === undefined) {
                  table[r.time] = {}
                  times.push(r.time)
                }
                table[r.time][name] = r.value
              })
            })
            times.sort((a, b) => a - b)
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
            const blob = new Blob(['\ufeff' + rows.join('\n')], {type: 'text/csv;charset=utf-8'})
            const url = URL.createObjectURL(blob)
            const a = document.createElement('a')
            a.href = url
            a.download = this.params.id + '-points' + this.dayjs().format('-YYYYMMDDHHmmss') + '.csv'
            document.body.appendChild(a)
            a.click()
            document.body.removeChild(a)
            URL.revokeObjectURL(url)
          })
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
            window: 10,
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
      start: this.dayjs().subtract(1, 'day').format('YYYY-MM-DD HH:mm:ss'),
      end: this.dayjs().format('YYYY-MM-DD HH:mm:ss'),
      window: 5,
      unit: 'm'
    }
    this.ensure_points(() => {
      //URL带point参数时预选该因子（从实时值卡片点入的场景），否则默认不选
      if (this.params.point) {
        try { this.toolbar.group.get('factor_sel').setValue([this.params.point]) } catch (e) { }
      }
      this.load_history()
    })
  },
  methods: {
    //加载产品物模型全部点位，填充因子下拉框
    ensure_points(cb) {
      const fallback = [{name: this.params.point || 'value', label: this.params.point || '值'}]
      const fillOptions = () => {
        this.content.toolbar.map(f => {
          if (f.key === 'factor_sel') {
            f.options = (this.points || []).map(p => {
              return {label: p.label ? p.label + '(' + p.name + ')' : p.name, value: p.name}
            })
          }
        })
      }
      const load = (pid) => {
        this.request.get('product/' + pid + '/setting/model').subscribe(res => {
          const points = []
          ;(res.data && res.data.content ? res.data.content : []).map(p => (p.points || []).map(pt => points.push(pt)))
          this.points = points.length ? points : fallback
          fillOptions()
          cb()
        })
      }
      if (this.params.product_id) {
        load(this.params.product_id)
      } else {
        this.request.get('table/device/detail/' + this.params.id).subscribe(res => {
          if (res.error || !res.data || !res.data.product_id) {
            this.points = fallback
            fillOptions()
            cb()
            return
          }
          load(res.data.product_id)
        })
      }
    },
    //按多选框选中的因子绘制：每因子独立纵轴自适应
    load_history() {
      const selNames = (this.toolbar.value && this.toolbar.value.factor_sel) || []
      const points = (this.points || []).filter(p => selNames.includes(p.name))
      const query = {
        start: this.dayjs(this.toolbar.value.start).toISOString(),
        end: this.dayjs(this.toolbar.value.end).toISOString(),
        window: this.toolbar.value.window + this.toolbar.value.unit,
        method: this.toolbar.value.method
      }
      if (!points.length) {
        //默认空图：显示已选因子或引导提示，点「查询」才绘制
        const text = '请在「选择因子」下拉框中选择因子（可多选），然后点击「查询」'
        this.chartOption = Object.assign({}, this.chartOption, {
          title: {text: text, left: 'center', top: 'middle', textStyle: {color: '#555', fontSize: 15, fontWeight: 'normal'}},
          xAxis: {type: 'time'},
          yAxis: {},
          legend: {data: []},
          grid: {left: 60, right: 40, top: 45, bottom: 40},
          series: []
        })
        this.mergeOption = {}
        this.render_data_table({}, [], [])
        return
      }
      Promise.all(points.map(p => {
        return new Promise(resolve => {
          this.request.get('device/' + this.params.id + '/history/' + p.name, query)
            .subscribe(res => resolve(res.data || []))
        })
      })).then(list => {
        const names = points.map(p => p.label ? p.label + '(' + p.name + ')' : p.name)

        //每因子一条独立纵轴，scale自适应数据区间，左右交替分布
        //数据直接内嵌在各series（[毫秒, 值]），time轴原生支持
        const n = points.length
        const nLeft = Math.ceil(n / 2)
        const axisNames = points.map(p => p.label || p.name)
        this.chartOption = Object.assign({}, this.chartOption, {
          title: {show: false},
          xAxis: {type: 'time'},
          legend: {data: names, top: 0},
          grid: {left: 60 + 40 * nLeft, right: 40 + 40 * (n - nLeft), top: 45, bottom: 40},
          yAxis: points.map((p, i) => {
            return {
              type: 'value',
              scale: true,
              name: axisNames[i],
              position: i < nLeft ? 'left' : 'right',
              offset: i < nLeft ? 36 * i : 36 * (i - nLeft),
              axisLine: {show: true},
              splitLine: {show: i === 0}
            }
          }),
          series: points.map((p, i) => {
            return {
              name: names[i],
              type: 'line',
              yAxisIndex: i,
              connectNulls: true,
              showSymbol: false,
              data: list[i].map(r => [r.time, r.value])
            }
          })
        })
        this.mergeOption = {}
        this.render_data_table(list, points, query)
      })
    },
    //渲染已选因子的数据表格（图表下方，时间对齐）
    render_data_table(list, points, query) {
      let el = document.getElementById('history-data-table')
      const host = document.querySelector('app-chart')
      if (!host) return
      if (!el) {
        el = document.createElement('div')
        el.id = 'history-data-table'
        el.style.margin = '12px 0'
        el.style.maxHeight = '380px'
        el.style.overflow = 'auto'
        host.appendChild(el)
      }
      if (!points || !points.length) {
        el.innerHTML = '<div style="text-align:center;color:#999;padding:16px">请先在上方选择因子，查询后此处显示数据表格</div>'
        return
      }
      //按时间戳对齐合并
      const table = {}
      const times = []
      list.forEach((records, idx) => {
        const name = points[idx].name
        records.map(r => {
          if (table[r.time] === undefined) {
            table[r.time] = {}
            times.push(r.time)
          }
          table[r.time][name] = r.value
        })
      })
      times.sort((a, b) => a - b)
      if (!times.length) {
        el.innerHTML = '<div style="text-align:center;color:#999;padding:16px">当前时间范围内没有数据</div>'
        return
      }
      const th = (t, sub) => '<th style="border:1px solid #e8e8e8;background:#fafafa;padding:8px 12px;white-space:nowrap;position:sticky;top:0">' + t + (sub ? '<br><small style="color:#888">' + sub + '</small>' : '') + '</th>'
      const td = (v) => '<td style="border:1px solid #e8e8e8;padding:6px 12px;white-space:nowrap">' + (v === null || v === undefined ? '' : v) + '</td>'
      let html = '<table style="border-collapse:collapse;width:100%;font-size:13px;text-align:center">'
      html += '<thead><tr>' + th('时间') + points.map(p => th(p.label || p.name, p.unit || '')).join('') + '</tr></thead><tbody>'
      const max = 500
      times.slice(0, max).map(t => {
        html += '<tr>' + td(this.dayjs(t).format('YYYY-MM-DD HH:mm:ss')) + points.map(p => td(table[t][p.name])).join('') + '</tr>'
      })
      html += '</tbody></table>'
      if (times.length > max) html += '<div style="text-align:center;color:#999;padding:8px">仅显示前 ' + max + ' 行（共 ' + times.length + ' 行），请用导出CSV获取全部数据</div>'
      el.innerHTML = html
    }
  }
}
