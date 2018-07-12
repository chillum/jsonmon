'use strict';

const gulp = require('gulp'),
      pug  = require('gulp-pug'),
      del  = require('del');

gulp.task('default', function() {
  return gulp.src('index.pug')
    .pipe(pug({pretty: true}))
    .pipe(gulp.dest('html'));
});

gulp.task('angular', function() {
  return gulp.src('node_modules/angular/angular.min.js')
    .pipe(gulp.dest('html'));
});

gulp.task('watch', function() {
  gulp.watch('index.pug', ['default']);
});

gulp.task('clean', function() {
  del('html/index.html');
});
