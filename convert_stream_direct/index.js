const gm = require('gm').subClass({imageMagick: true});
var request = require('request');
var express = require('express');

const { Storage } = require('@google-cloud/storage');
const storage = new Storage();

var app = express();

const gcsBucketName = 'mineral-minutia-820-img';
const exprieInMinutes = 3;

const bucketName = process.env.GCS_BUCKET || gcsBucketName;

app.get('/images/:fileName', async (req, res) => {    
    const b = storage.bucket(bucketName);
    const f = b.file(req.params.fileName);
    
    // Does a metadata lookup here.  Adds some latency so maybe skip this?
    const [metadata] = await f.getMetadata();
    res.setHeader("Content-Type", metadata.contentType);
    res.setHeader("Cache-Control", metadata.cacheControl);

    // if no &l=&w= are set, transform otherwise just transmit it as-is
    if (req.query.w && req.query.l) {
      let stream = f.createReadStream()
      stream.on('error', function(err) {
        console.error(err);
        res.sendStatus(err.code).end(err);
      });
      gm(stream)
        .resize(req.query.w,req.query.h)
        .stream()
        .pipe(res);              
    } else {
      f.createReadStream()
        .on('error', function(err) {
          res.setHeader("content-type", "text/plain");
          res.status(500);
        })
        .pipe(res);
    }
});


app.listen(8080, function () {
	console.log('listening on port 8080!');
});
