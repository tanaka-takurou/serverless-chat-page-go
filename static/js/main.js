$(window).on('load', function() {
  ScrollMessageBottom();
});

var webSocket = null;

function init() {
  $("#chat_send_message").keypress(press);
  open();
}

function open() {
  if (webSocket == null) {
    webSocket = new WebSocket(App.url);
    webSocket.onopen = onOpen;
    webSocket.onmessage = onMessage;
    webSocket.onclose = onClose;
    webSocket.onerror = onError;
  }
}

function onOpen(event) {
  console.log('Join');
}

function onMessage(event) {
  if (event && event.data) {
    var res = JSON.parse(event.data);
    chat(res.data , res.color, false);
  }
}

function onError(event) {
  chat("Error", 'F00', false);
  console.log('Error. Wait a minute please.');
}

function onClose(event) {
  console.log('onClose');
  webSocket = null;
}

function press(event) {
  if (event && event.which == 13) {
    send();
  }
}

function OnClickSend(event) {
  send();
}

function send() {
  var message = $("#chat_send_message").val();
  console.log(message);
  if (message && webSocket) {
    var obj = new Object();
    obj.text = message;
    obj.image = '';
    obj.action = 'send';
    var jsonString = JSON.stringify(obj);
    webSocket.send(jsonString);
    $("#chat_send_message").val("");
    chat(message, '00F', true);
  }
}

function chat(message, col, slf) {
  var chats = $("#chat_messages").find("div");
  while (chats.length >= App.maxMessage) {
    chats = chats.first().remove();
  }
  var itemClassName = "item"
  if (slf) {
    itemClassName = itemClassName + " self";
  }
  var msgTag;
  if (CheckBucketName(message)) {
    imgTag = $("<img>", {
      "src": message
    });
    msgTag = $("<div></div>", {
      "class": "content"
    }).append(imgTag);
  } else {
    msgTag = $("<div></div>", {
      "class": "content"
    }).text(message);
  }
  var iconTag = $("<i></i>", {
    "class": "large user middle aligned icon",
    "style": "color: #" + col
  });
  var msgItemTag = $("<div></div>", {
    "class": itemClassName
  }).append(iconTag).append(msgTag);
  $("#chat_messages").append(msgItemTag);
  ScrollMessageBottom();
}
function OpenModal() {
  $('.large.modal').modal('show');
}
function CloseModal() {
  $('.large.modal').modal('hide');
}
function toBase64 (file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.readAsDataURL(file);
    reader.onload = () => resolve(reader.result);
    reader.onerror = error => reject(error);
  });
}
function onConverted () {
  return function(v) {
    App.imgdata = v;
    $('#preview').attr('src', v);
  }
}
function UploadImage(elm) {
  if (!!App.imgdata) {
    $(elm).addClass("disabled");
    putImage();
  } else {
    CloseModal();
  }
}
function putImage() {
  const file = $('#image').prop('files')[0];
  console.log(file.name);
  if (file && App.imgdata && webSocket) {
    var obj = new Object();
    obj.text = file.name;
    obj.image = App.imgdata;
    obj.action = 'send';
    var jsonString = JSON.stringify(obj);
    webSocket.send(jsonString);
    $("#chat_send_message").val("");
    CloseModal();
  }
}
function ChangeImage() {
  const file = $('#image').prop('files')[0];
  toBase64(file).then(onConverted());
}
function CheckBucketName(str) {
  var search = 'https://' + App.bucketName
  if (search.length > str.length){
    return false
  }
  return str.substring(0, search.length) === search
}
function ScrollMessageBottom() {
  var target = $("#chat_messages");
  target.scrollTop(target.get(0).scrollHeight - target.get(0).offsetHeight);
}
var App = { imgdata: null, url: {{ .Url }}, maxMessage: {{ .Max }}, bucketName: {{ .Bucket }} };
$(init);
