'use strict';

const fs       = require('fs'),
      yaml     = require('js-yaml'),
      format   = require('dateformat'),
      run      = require('child_process').exec,
      http     = require('http'),
      https    = require('https'),
      mailer   = require('nodemailer'),
      sendmail = require('nodemailer-sendmail-transport'),
      Hapi     = require('hapi');

// Global failed/succeeded maps.
var failed = {}, ok = {};
// Sendmail transport.
const send = mailer.createTransport(sendmail({path: '/usr/sbin/sendmail'}));
// The JSON API.
const server = new Hapi.Server();

// Logs and mail alerting.
const alert = function(mail, subject, message) {
  let rcpt;
  if (mail) {
    if (Array.isArray(mail))
      rcpt = mail.join(', ');
    else
      rcpt = mail;
    // Send alerts by mail.
    send.sendMail({
      to: rcpt,
      subject: subject,
      text: message
    }, function(error){
      if (error)
        console.warn(error);
    });
  }
  console.log(subject); // And log them.
  if (message) console.log(message);
};

// Mark Web check as failed.
const fail = function(name, url, notify, message) {
  if (!failed[url]) failed[url] = format(Date.now(), 'isoDateTime');
  if (ok[url]) {
    delete ok[url];
    alert(notify, 'FAILED: ' + (name || url), message);
  }
};

// Fail if HTTP status code is not 200.
const check = function(name, url, req, notify, tries) {
  if (req.statusCode !== 200) {
    if (tries)
      check(name, url, req, notify, tries - 1);
    else
      fail(name, url, notify, 'Error: ' + url + ' returned ' + req.statusCode);
  } else {
    if (failed[url]) {
      delete failed[url];
      ok[url] = format(Date.now(), 'isoDateTime');
      alert(notify, 'FIXED: ' + (name || url));
    }
  }
  req.socket.destroy();
};

// Check the HTTP URL in several tries.
const fetch_http = function(name, url, notify, tries) {
  http.get(url, function(res) {
    check(name, url, res, notify, tries);
  }).on('error', function(e) {
    if (tries)
      fetch_http(name, url, notify, tries - 1);
    else
      fail(name, url, notify, e.toString());
  });
};

// Check the HTTPS URL in several tries.
const fetch_https = function(name, url, notify, tries) {
  https.get(url, function(res) {
    check(name, url, res, notify, tries);
  }).on('error', function(e) {
    if (tries)
      fetch_https(name, url, notify, tries - 1);
    else
      fail(name, url, notify, e.toString());
  });
};

// Web worker.
const web = function(name, url, notify, repeat) {
  if (url.match(/^http:\/\//)) {
    fetch_http(name, url, notify, 3);
  } else if (url.match(/^https:\/\//)) {
    fetch_https(name, url, notify, 3);
  } else {
    // Failed URL: non HTTP or HTTPS.
    fail(name, url, notify, 'Error: ' + url + ' is not a valid HTTP(S) URL');
    return;
  }
  setTimeout(function() {
    web(name, url, notify, repeat);
  }, repeat * 1000);
};

// Shell worker.
const exec = function(name, cmd, notify, repeat) {
  run(cmd, function(error, stdout, stderr) {
    if (error !== null) {
      if (!failed[cmd]) failed[cmd] = format(Date.now(), 'isoDateTime');
      if (ok[cmd]) {
        delete ok[cmd];
        alert(notify, 'FAILED: ' + (name || cmd), error.toString().trim());
      }
    } else {
      if (failed[cmd]) {
        delete failed[cmd];
        ok[cmd] = format(Date.now(), 'isoDateTime');
        alert(notify, 'FIXED: ' + (name || cmd));
      }
    }
    setTimeout(function() {
      exec(name, cmd, notify, repeat);
    }, repeat);
  });
};

// Format JSON for output.
const display = function(i) {
  let o = {};
  if (i.name)
    o.name = i.name;
  if (i.web) {
    o.web = i.web;
    if (failed[i.web]) {
      o.failed = true;
      o.since = failed[i.web];
    } else {
      o.failed = false;
      if (ok[i.web] !== true)
        o.since = ok[i.web];
    }
  }
  if (i.shell) {
    o.shell = i.shell;
    if (failed[i.shell]) {
      o.failed = true;
      o.since = failed[i.shell];
    } else {
      o.failed = false;
      if (ok[i.shell] !== true)
        o.since = ok[i.shell];
    }
  }
  return o;
};

// The main loop.
const main = function() {
  var checks;
  // Get config or throw exception on error.
  try {
    checks = yaml.safeLoad(fs.readFileSync('config.yml', 'utf8'));
  } catch (e) {
    console.warn(e);
    process.exit(1);
  }
 
  // Run checks.
  checks.forEach(function(i) {
    if (!i.repeat)
      i.repeat = 60;
    if (i.web) {
      ok[i.web] = true;
      web(i.name, i.web, i.notify, i.repeat);
    }
    if (i.shell) {
      ok[i.shell] = true;
      exec(i.name, i.shell, i.notify, i.repeat);
    }
  });

  // Launch the JSON API.
  server.connection({
    host: (process.env.HOST || 'localhost'),
    port: (process.env.PORT || '3000')
  });
  server.route({
    method: 'GET',
    path: '/',
    handler: function(request, reply) {
      let result = [];
      checks.forEach(function(i) {
        result.push(display(i));
      });
      reply(JSON.stringify(result, null, 2)).header('Content-type', 'application/json');
    }
  });
  server.start();
};
main();
