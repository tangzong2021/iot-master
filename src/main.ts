import {bootstrapApplication} from '@angular/platform-browser';
import {appConfig} from './app/app.config';
import {AppComponent} from './app/app.component';
import * as XLSX from 'xlsx';

//暴露给 pages 配置脚本使用(如历史曲线页导出Excel)
(window as any).XLSX = XLSX;

bootstrapApplication(AppComponent, appConfig)
  .catch((err) => console.error('bootstrap:', err));
