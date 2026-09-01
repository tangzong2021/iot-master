#!/bin/bash
# 同步固件版本页面到线上 PVC (AGENTS.md 记录的发布流程)
# 用法: 在能连 kubectl 的环境执行; 先改好 NS/POD 两个变量
# 依赖: 本目录下 pages/version.js version_create.js version_edit.js
set -e

NS="ns-y5hvuko4"
# kubectl get pod -n $NS | grep iot-master 找到实际 pod 名
POD="iot-master-rdnuosuk-0"
CONTAINER="iot-master"   # 若 pod 只有一个容器可不改

cd "$(dirname "$0")"

for f in version.js version_create.js version_edit.js; do
  echo "同步 pages/$f ..."
  base64 -w0 "pages/$f" | kubectl exec -i -n "$NS" "$POD" -c "$CONTAINER" -- \
    sh -c "base64 -d > /app-data/pages/$f"
done

echo "重启 pod 使页面生效..."
kubectl delete pod -n "$NS" "$POD"
echo "完成。pod 会自动重建(数据在PVC不受影响), 等1-2分钟后刷新平台页面验证。"
