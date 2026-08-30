// 实时数据页面配置
return {
  title: '实时数据',
  template: 'detail',
  toolbar: [
    {
      type: 'button',
      label: '采集数据',
      icon: 'reload',
      action: {
        type: 'script',
        script(data, index) {
          if (this.params.gateway_id) this.refresh_child_values()
          else this.refresh_values()
        }
      }
    },
    {
      type: 'button',
      label: '修改数据',
      icon: 'edit',
      action: {
        type: 'dialog',
        page: 'device_values_setting',
        params(data) {
          return { id: this.params.id }
        }
      }
    }
  ],
  items: [],
  auto_refresh: 10,
  load_api: 'device/:id/values',
  load_success(data) {
    this.render_values()
  },
  // 页面挂载时执行
  mount() {
    this.load_model(this.params.product_id)
  },
  methods: {
    load_values() {
      this.request.get('device/' + this.params.id + '/values').subscribe(res => {
        if (res.error) return
        //数据时间格式化（断网补发时显示的是设备采集时间）
        if (res.data && res.data._update) {
          const t = new Date(res.data._update)
          if (!isNaN(t.getTime())) {
            res.data._update = t.getFullYear() + '-' + String(t.getMonth() + 1).padStart(2, '0') + '-' + String(t.getDate()).padStart(2, '0') + ' ' + String(t.getHours()).padStart(2, '0') + ':' + String(t.getMinutes()).padStart(2, '0') + ':' + String(t.getSeconds()).padStart(2, '0')
          }
        }
        this.data = res.data
      })
    },
    load_values_delay(delay) {
      setTimeout(() => this.load_values(), delay || 1000)
    },
    refresh_values() {
      this.request.get('device/' + this.params.id + '/sync').subscribe(res => {
        if (res.error) return
        this.load_values_delay()
      })
    },
    refresh_child_values() {
      this.request.get('device/' + this.params.gateway_id + '/sync/' + this.params.id).subscribe(res => {
        if (res.error) return
        this.load_values_delay()
      })
    },
    load_model(pid) {
      this.request.get('product/' + pid + '/setting/model').subscribe(res => {
        if (res.error) return
        if (res.data.content) this.render_properties(res.data.content)
        setTimeout(() => this.render_values(), 100)
      })
    },
    render_properties(properties) {
      //数据时间卡片置顶（显示本批数据的采集时间）
      this.content.children.unshift({
        span: 24,
        content: {
          title: '数据时间',
          template: 'statistic',
          fields: [
            {key: '_update', label: '采集时间'}
          ]
        }
      })
      if (properties) {
        properties.map(p => {
          this.content.children.push({
            span: 24,
            content: {
              title: p.name,
              template: 'statistic',
              fields: this.render_points(p.points)
            }
          })
        })
      }
    },
    render_points(points) {
      if (!points) return []
      return points.map(p => {
        return {
          key: p.name,
          label: p.label,
          suffix: p.unit,
          action: {
            type: 'dialog',
            page: 'device_history',
            params: { id: this.params.id, point: p.name }
          }
        }
      })
    },
    render_values() {
      this.pageComponent.children.map(p => {
        p.componentRef.setInput('data', this.data || {})
      })
    }
  },
  children: []
}
