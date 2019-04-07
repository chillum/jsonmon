'use strict';

const { task, series,
  src, dest, watch } = require('gulp'),
  pug                = require('gulp-pug'),
  del                = require('del');

task('default', function() {
  return src('index.pug')
    .pipe(pug({pretty: true}))
    .pipe(dest('html'));
});

task('angular', function() {
  return src('node_modules/angular/angular.min.js')
    .pipe(dest('html'));
});

task('watch', function() {
  watch('index.pug', series('default'));
});

task('clean', function(done) {
  del.sync('html/index.html');
  done();
});
