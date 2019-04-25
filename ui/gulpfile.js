'use strict';

const { task, series, parallel,
  src, dest, watch } = require('gulp'),
  pug                = require('gulp-pug');

task('html', function() {
  return src('index.pug')
    .pipe(pug({pretty: true}))
    .pipe(dest('html'));
});

task('angular', function() {
  return src('node_modules/angular/angular.min.js')
    .pipe(dest('html'));
});

task('default', parallel('angular', 'html'));

task('watch', function() {
  watch('index.pug', series('html'));
  watch('node_modules/angular/angular.min.js', series('angular'));
});
