// Package common holds the frontend assets shared by the single and multi apps.
package common

import "embed"

//go:embed style.css common.js
var Assets embed.FS
