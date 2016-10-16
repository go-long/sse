//-------- EventSource ------------------------
var source = new EventSource('/events/');
//Create a callback for when a new message is received.
source.onmessage = getMessage;
//------------------------------
function postMessage() {
	var xhr = new XMLHttpRequest();
	var text = document.getElementById("text").value;
    // Check textbox on empty
    if (text != "") {
    	var statusMsg = document.getElementById('statusMsg');
    	// Make request
    	xhr.open('POST', '/message', true);
    	xhr.setRequestHeader('Content-Type', 'application/json; charset=UTF-8');
    	xhr.send(JSON.stringify({
    	    message: text,
    	}));
        // callback call
    	xhr.onreadystatechange = function() {
    	    if (xhr.readyState != 4) {
    	        return;
    	    }
    	    statusMsg.innerHTML = "";
            // Fail send
    	    if (xhr.status != 200) {
    	        console.log(xhr.status + ': ' + xhr.statusText);
    	    } else {
                // Clear textbox
                document.getElementById("text").value = ""
            }
    	}
    	statusMsg.innerHTML = 'sent...';
	}
}
function getMessage(respone) {
	console.log(respone)
    // Check count message
    var areaChat = document.getElementById('areaChat');
    var text = respone.data + '<br>';
    areaChat.innerHTML += text;
}
