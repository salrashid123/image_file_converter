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
    console.log(metadata);    
    res.setHeader("Cache-Control", metadata.cacheControl);

    // if no query params are provided, just transmit it as-is
    if (Object.keys(req.query).length  == 0 ) {
      f.createReadStream()
        .on('error', function(err) {
          res.setHeader("content-type", "text/plain");
          res.status(500);
        })
        .pipe(res);
    }
    else {
      // otherwise resize it with &l=&w= parameter
      let stream = f.createReadStream()
      stream.on('error', function(err) {
        console.error(err);
        res.sendStatus(err.code).end(err);
      });

      gm(stream)
        .resize(req.query.w,req.query.h)
        .stream()
        .pipe(res);      
    }
});


app.listen(8080, function () {
	console.log('listening on port 8080!');
});
