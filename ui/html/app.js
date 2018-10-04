'use strict';

var App   = angular.module('jsonmon', []),
    Title = 'Systems status';

App.config(['$compileProvider', function($compileProvider) {
  $compileProvider.debugInfoEnabled(false);
}]);

function getJson($rootScope, $scope, $http) {
  $http.get('/status')
    .then(function(res){
      if (!angular.equals($scope.json, res.data)) {
        $scope.json = res.data;
        // Page title should include errors number.
        var errors = res.data.filter(function(check) {
          return check.failed;
        });
        if (errors.length) {
          $rootScope.title = '(' + errors.length + ') ' + Title;
        } else {
          $rootScope.title = Title;
        }
      }
    });
}

App.controller('reload', function($rootScope, $scope, $http) {
  getJson($rootScope, $scope, $http);
  setInterval(function() {
    getJson($rootScope, $scope, $http);
  }, 5 * 1000);
});
