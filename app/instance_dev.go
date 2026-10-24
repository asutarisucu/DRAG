//go:build dev

package main

// instanceID は、2つ目の起動を今の窓に回すための名前。
// wails dev では別の名前にして、普段使いの DRAG と並べて動かせるようにする
const instanceID = "io.github.asutarisucu.drag.dev"

// configName は、設定と記録を置くフォルダの名前。
// wails dev で試すときに、普段使いの DRAG の保存済みのサーバーやスマホの登録に触らないよう、別のフォルダにする
const configName = "DRAG-dev"
