// 编辑版本页面配置
return {
  title: '编辑版本',
  icon: '/emoji/version.svg',
  template: 'edit',
  fields: [
    { key: 'name', label: '名称(版本号)', type: 'text', required: true, placeholder: '合宙格式1122.1122.001，最后一段为脚本版本递增，须与设备VERSION一致' },
    { key: 'description', label: '说明', type: 'text' },
    { key: 'url', label: '固件', type: 'file', upload: '/api/upload' },
    {
      key: 'product_id',
      label: '产品ID',
      type: 'text',
      link_text: '选择',
      link_action: {
        type: 'dialog',
        page: 'product_choose',
        after_close(result, data, index) {
          this.editor.patchValue({ product_id: result.id })
          this.content.fields[3].tips = result.name
        }
      }
    },
    { key: 'disabled', label: '禁用', type: 'switch' }
  ],
  load_api: 'table/version/detail/:id',
  submit_api: 'table/version/update/:id',
  load_success() {
    this.content.fields[3].tips = this.data.product_name
  }
}
