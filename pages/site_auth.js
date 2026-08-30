// 站点授权页面配置：给指定用户分配可查看的站点（设备）
return {
  title: '站点授权',
  icon: '/emoji/device.svg',
  template: 'detail',
  toolbar: [
    {
      type: 'button',
      label: '保存授权',
      icon: 'save',
      action: {
        type: 'script',
        script(data, index) {
          this.save_auth()
        }
      }
    },
    {
      type: 'link',
      label: '返回',
      action: {
        type: 'script',
        script(data, index) {
          history.back()
        }
      }
    }
  ],
  mount() {
    const uid = this.params.id
    if (!uid) {
      alert('缺少用户参数')
      return
    }
    let done = 0
    const devices = []
    const bindings = {}
    const tryRender = () => {
      if (++done < 2) return
      this.render_auth(uid, devices, bindings)
    }

    //全部设备
    this.request.post('table/device/search', {limit: 999}).subscribe(res => {
      if (!res.error) (res.data || []).map(d => devices.push(d))
      tryRender()
    })
    //已有绑定
    this.request.post('table/user_site/search', {limit: 999, filter: {user_id: uid}}).subscribe(res => {
      if (!res.error) (res.data || []).map(b => bindings[b.site_id] = b)
      tryRender()
    })
  },
  methods: {
    render_auth(uid, devices, bindings) {
      //用户名
      this.request.get('table/user/detail/' + uid).subscribe(res => {
        const el = document.getElementById('site-auth-box')
        if (el && res.data) {
          const info = el.querySelector('.auth-user-info')
          if (info) info.textContent = '授权用户：' + (res.data.name || uid) + '（' + uid + '）'
        }
      })

      let el = document.getElementById('site-auth-box')
      if (!el) {
        el = document.createElement('div')
        el.id = 'site-auth-box'
        el.style.padding = '8px 16px'
        const host = document.querySelector('app-detail') || document.body
        host.appendChild(el)
      }

      const box = document.createElement('div')
      box.className = 'auth-user-info'
      box.style.cssText = 'font-size:15px;font-weight:bold;padding:8px 0'
      box.textContent = '授权用户：' + uid
      el.innerHTML = ''
      el.appendChild(box)

      const list = document.createElement('div')
      list.style.cssText = 'border:1px solid #e8e8e8;border-radius:4px;max-height:480px;overflow:auto;padding:8px'
      devices.map(d => {
        const id = 'sa-' + d.id
        const label = (d.name || d.id) + '（' + d.id + '）' + (d.product_name ? ' [' + d.product_name + ']' : '')
        const row = document.createElement('label')
        row.style.cssText = 'display:flex;align-items:center;gap:8px;padding:6px 8px;cursor:pointer'
        row.innerHTML = '<input type="checkbox" id="' + id + '"' + (bindings[d.id] ? ' checked' : '') + '>' +
          '<span>' + label + '</span>' +
          (bindings[d.id] ? '<small style="color:#52c41a">已授权</small>' : '')
        list.appendChild(row)
      })
      if (!devices.length) list.innerHTML = '<div style="color:#999;padding:12px">没有可选站点</div>'
      el.appendChild(list)
      el.dataset.uid = uid
    },
    save_auth() {
      const el = document.getElementById('site-auth-box')
      if (!el) return
      const uid = el.dataset.uid
      if (!uid) return
      //收集当前勾选
      const checked = []
      el.querySelectorAll('input[type=checkbox]').forEach(cb => {
        if (cb.checked) checked.push(cb.id.slice(3))
      })
      //与现有绑定对比，增删
      this.request.post('table/user_site/search', {limit: 999, filter: {user_id: uid}}).subscribe(res => {
        const old = {}
        ;(res.data || []).map(b => old[b.site_id] = b)
        const toAdd = checked.filter(sid => !old[sid])
        const toDel = Object.keys(old).filter(sid => !checked.includes(sid))
        let total = toAdd.length + toDel.length
        if (!total) {
          this.notification.success('提示', '站点授权已保存')
          return
        }
        const done = () => {
          if (--total <= 0) this.notification.success('提示', '站点授权已保存')
        }
        toAdd.map(sid => {
          this.request.post('table/user_site/create', {
            id: 'us-' + sid + '-' + uid,
            user_id: uid,
            site_id: sid,
            site_name: (devices.find(d => d.id === sid) || {}).name || sid
          }).subscribe(done)
        })
        toDel.map(sid => {
          this.request.post('table/user_site/delete/' + old[sid].id, {}).subscribe(done)
        })
      })
    }
  }
}
