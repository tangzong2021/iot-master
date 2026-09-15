// 实时数据页面配置：全因子数据表（可选时间范围查询，缺测留空）+ 数据曲线入口
// 原分组卡片视图已移除（与历史曲线页数据表重复），本页即"选时间戳看数据"的主视图
return {
  title: '实时数据',
  template: 'detail',
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
      type: 'button',
      label: '查询',
      action: {
        type: 'script',
        script(data, index) {
          this.load_table()
        }
      }
    },
    {
      type: 'button',
      label: '数据曲线',
      icon: 'line-chart',
      action: {
        type: 'script',
        script(data, index) {
          this.navigate('/page/device_history?id=' + this.params.id)
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
          setTimeout(() => this.load_table(), 100)
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
          setTimeout(() => this.load_table(), 100)
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
          setTimeout(() => this.load_table(), 100)
        }
      }
    }
  ],
  items: [],
  // 页面挂载时执行
  mount() {
    this.toolbarValue = {
      start: this.dayjs().subtract(1, 'day').format('YYYY-MM-DD HH:mm:ss'),
      end: this.dayjs().format('YYYY-MM-DD HH:mm:ss'),
      window: 5,
      unit: 'm'
    }
    this.ensure_points(() => this.load_table())
  },
  methods: {
    //按物模型点位精度格式化数值显示(未配precision或非数值则原样返回)
    fmt_point(v, p) {
      if (v === null || v === undefined || v === '') return v
      const n = Number(v)
      if (isNaN(n)) return v
      const pr = p && p.precision
      if (pr === undefined || pr === null || pr === '' || isNaN(Number(pr))) return v
      return n.toFixed(Number(pr))
    },
    //加载产品物模型全部点位
    ensure_points(cb) {
      const fallback = [{name: 'value', label: '值'}]
      const load = (pid) => {
        this.request.get('product/' + pid + '/setting/model').subscribe(res => {
          const points = []
          ;(res.data && res.data.content ? res.data.content : []).map(p => (p.points || []).map(pt => points.push(pt)))
          this.points = points.length ? points : fallback
          cb()
        })
      }
      if (this.params.product_id) {
        load(this.params.product_id)
      } else {
        this.request.get('table/device/detail/' + this.params.id).subscribe(res => {
          if (res.error || !res.data || !res.data.product_id) {
            this.points = fallback
            cb()
            return
          }
          load(res.data.product_id)
        })
      }
    },
    //查询：拉取时间范围内全部因子，渲染数据表（最新时刻在最上面）
    load_table() {
      const allPoints = this.points || []
      //工具栏表单值可能尚未同步(挂载时序)，空则回退到 mount 设置的默认值，避免拼出 window=NaN
      const fv = (this.toolbar && this.toolbar.value) || {}
      const tv = (fv.start || fv.end) ? fv : (this.toolbarValue || {})
      const start = tv.start || this.dayjs().subtract(1, 'day').format('YYYY-MM-DD HH:mm:ss')
      const end = tv.end || this.dayjs().format('YYYY-MM-DD HH:mm:ss')
      const win = Number(tv.window) > 0 ? Number(tv.window) : 5
      const unit = ['s', 'm', 'h', 'd'].indexOf(tv.unit) >= 0 ? tv.unit : 'm'
      const query = {
        start: this.dayjs(start).toISOString(),
        end: this.dayjs(end).toISOString(),
        window: win + unit,
        method: 'last'
      }
      if (!allPoints.length) {
        this.render_table([], [])
        return
      }
      Promise.all(allPoints.map(p => {
        return new Promise(resolve => {
          this.request.get('device/' + this.params.id + '/history/' + p.name, query)
            .subscribe(res => resolve(res.data || []))
        })
      })).then(list => this.render_table(list, allPoints))
    },
    //渲染数据表：全部因子铺列，时间倒序（最新在最上），缺失留空
    //本页作为设备详情子Tab时页面里有两层app-detail，必须锚定最内层(本页自己的)，
    //且表格放到工具栏卡片之后(区块底部)
    render_table(list, points) {
      let el = document.getElementById('rt-data-table')
      const hosts = document.querySelectorAll('app-detail')
      const host = hosts.length ? hosts[hosts.length - 1] : null
      if (!host) return
      if (!el) {
        el = document.createElement('div')
        el.id = 'rt-data-table'
        el.style.cssText = 'margin:8px 0 0;max-height:60vh;overflow:auto;background:#fff;padding:8px'
        host.appendChild(el)
      }
      if (!points || !points.length) {
        el.innerHTML = '<div style="text-align:center;color:#999;padding:16px">当前产品没有物模型点位</div>'
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
      let html = '<div style="color:#666;padding:4px 2px">设备: ' + (this.params.id || '-') + '　共 ' + times.length + ' 个时刻（最新在最上，缺测留空；需要曲线请点「数据曲线」）</div>'
      html += '<table style="border-collapse:collapse;width:100%;font-size:13px;text-align:center">'
      html += '<thead><tr>' + th('数据时间') + points.map(p => th(p.label || p.name, p.unit || '')).join('') + '</tr></thead><tbody>'
      const max = 500
      times.slice().reverse().slice(0, max).map(t => {
        html += '<tr>' + td(this.dayjs(t).format('YYYY-MM-DD HH:mm:ss')) + points.map(p => td(this.fmt_point(table[t][p.name], p))).join('') + '</tr>'
      })
      html += '</tbody></table>'
      if (times.length > max) html += '<div style="text-align:center;color:#999;padding:8px">仅显示最新 ' + max + ' 行（共 ' + times.length + ' 行），请缩小时间范围查看更早数据</div>'
      el.innerHTML = html
    }
  }
}
