#!/bin/bash

read -p "commit信息: " msg

if [ -z "$msg" ]; then
  echo "commit信息不能为空"
  exit 1
fi

git add .
git commit -m "$msg"
git push
