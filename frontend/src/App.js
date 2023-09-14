import './App.css';

import { useState, useEffect } from "react"; 

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
} from 'chart.js';
import { Line } from 'react-chartjs-2';
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend
);

const server = "http://localhost:8080/";

async function fetchLinks() {
  const response = await fetch(server + "api/getLinks", {
    method: "GET",
    credentials: "include"
  });
  const links = await response.text();
  if (links.length > 0)
    return links.split(" ");
  else
    return []
}

function Statistics({history, setHistory}) {
  const [links, setLinks] = useState([])
  const [selectedLinkIndex, setSelectedLinkIndex] = useState(0)

  const [longURL, setLongURL] = useState("");
  const [totalNumClicks, setTotalNumClicks] = useState(0)
  const [chartData, setChartData] = useState({
    labels: [],
    datasets: [],
  });

  useEffect(() => {
    fetchLinks().then(result => {
      if (result.length > 0) {
        setLinks(result)
      }
    });
  }, [])

  useEffect(() => {
    if (history.length > 0) {
      setLinks(
        [...links,
         history[history.length - 1].short]
    );
    }
    
  }, [history])

  const refreshData = () => {
    if (links.length === 0) return;

    fetch(server + "api/getLinkData", {
      method: "POST",
      credentials: "include",
      body: links[selectedLinkIndex]
    })
    .then(response => response.json())
    .then(data => {
      setLongURL(data.LongURL);

      const labels = [];
      for (let i = 29; i >= 2; --i) {
        labels.push("+" + i);
      }
      labels.push("igår")
      labels.push("idag")

      let totalNum = 0;
      for (let i = 0; i < 30; ++i) {
        totalNum += data.NumClicks[i];
      }
      setTotalNumClicks(totalNum);

      setChartData({
        labels,
        datasets: [
          {
            data: data.NumClicks.reverse(),
            borderColor: 'rgb(0, 99, 132)',
            backgroundColor: 'rgba(255, 99, 132, 0.5)',
          },
        ],
      });
    })
  }

  useEffect(() => {
    refreshData();
  }, [links, selectedLinkIndex])

  const options = {
    responsive: true,
    plugins: {
      legend: {
        display: false,
      },
      title: {
        display: true,
        text: "Senaste 30 dagarna",
      },
    },
  };

  const removeLink = e => {
    fetch(server + "api/removeLink", {
      method: "POST",
      credentials: "include",
      body: links[selectedLinkIndex]
    });
    setHistory(history.filter(link => link.short != links[selectedLinkIndex]))
    setLinks(links.filter((link, i) => i != selectedLinkIndex))
    setSelectedLinkIndex(0);
  }

  return (
    <div id="statistics">
      <h2>Statistik</h2>
      <div id="statistics-inner">
        <div>
          <h3>Dina länkar:</h3>
          <ul >
            {links.map((link, i) => (
              <li key={link} onClick={e => setSelectedLinkIndex(i)} id={selectedLinkIndex === i ? "selected" : 0}>{link}</li>
            ))}
          </ul>
        </div>
        {links.length > 0 &&
        <div>
          <h3>Data:</h3>
          <button onClick={refreshData}>UPPDATERA</button>
          <button onClick={removeLink}>TA BORT</button>
          <br/>
          Kort: <a target="_blank" href={server + links[selectedLinkIndex]}>{links[selectedLinkIndex]}</a>
          <br/>
          Lång: <a target="_blank" href={longURL}>{longURL}</a>

          <p>Antal klick: {totalNumClicks}</p>
          <Line options={options} data={chartData} width={600} height={300}/>
        </div>}
      </div>
    </div>
  )
}

function App() {

  const [longURL, setLongURL] = useState("");
  const [serverError, setServerError] = useState("");
  const [history, setHistory] = useState([]);
  const [loggedInUsername, setLoggedInUsername] = useState("")

  fetch(server + "api/getSessionUsername",
  {
    method: "POST",
    credentials: "include",
  })
  .then(response => {
    if (response.ok) {
      return response.text();
    }
    return Promise.reject(response);
  })
  .then(username => setLoggedInUsername(username))
  .catch(() => {});

  function shortenURL(e) {
    e.preventDefault();

    setServerError("");

    fetch(server + "api/shorten", {
      method: "POST",
      credentials: "include",
      body: longURL
    })
    .then(response => { 
      if (response.ok) {
        return response.text();
      }
      return Promise.reject(response);
    })
    .then(response_text => {
      let modifiedLongURL = longURL;
      const scheme_end_index = longURL.indexOf(":");
      if (scheme_end_index === -1) {
        modifiedLongURL = "http://" + longURL;
      }

      setHistory([...history,
                  {
                    short: response_text,
                    long: modifiedLongURL,
                  }]);
    })
    .catch(response => response.text()
      .then(errorText => setServerError("Failed to shorten URL: " + errorText)))
  }

  function tryLogin(e) {
    e.preventDefault();

    const username = document.querySelector("#username").value;
    const password = document.querySelector("#password").value;
    if (username.length == 0 || password.length == 0) return;

    fetch(server + "api/login", {
      method: "POST",
      credentials: "include",
      body: JSON.stringify({
        username: username,
        password: password,
      })
    })
    .then(response => {
      if (response.ok) {
        setLoggedInUsername(username)
        setServerError("");
      } else {
        return Promise.reject(response);
      }
    })
    .catch(response => response.text()
      .then(errorText => setServerError(errorText)));
  }

  function tryRegister(e, setShowRegister) {
    e.preventDefault();

    const username = document.querySelector("#username").value;
    const password = document.querySelector("#password").value;

    fetch(server + "api/register", {
      method: "POST",
      body: JSON.stringify({
        username: username,
        password: password,
      })
    })
    .then(response => {
      if (response.ok) {
        return response.text();
      }
      return Promise.reject(response);
    })
    .then(response_text => {
      setServerError("");
      setShowRegister(false)
    })
    .catch(response => response.text()
      .then(errorText => setServerError(errorText)));
  }

  function LoginSection() {
    const [showRegister, setShowRegister] = useState(false)

    const switchShowRegister = e => {
      e.preventDefault();
      setShowRegister(!showRegister);
    }

    const logoutClick = e => {
      e.preventDefault();
      fetch(server + "api/logout", {
        method: "POST",
        credentials: "include",
      });
      setLoggedInUsername("");
    }

    return (
      <div id="loginSection">
        {loggedInUsername.length === 0 && <>
          <a href="/" onClick={switchShowRegister}>{showRegister ? "Logga in" : "Registrera dig"}</a>

          {showRegister === false && 
          <form autoComplete="off" onSubmit={tryLogin}>
            <input type="text" id="username" placeholder="Användarnamn"></input>
            <input type="password" id="password" placeholder="Lösenord"></input>
            <button>LOGGA IN</button> 
          </form>
          }
          {showRegister === true &&
          <form autoComplete="off" onSubmit={e => tryRegister(e, setShowRegister)}>
            <input type="text" id="username" placeholder="Användarnamn"></input>
            <input type="password" id="password" placeholder="Lösenord"></input>
            <button>REGISTRERA</button> 
          </form>
          }
        </>}
        {loggedInUsername.length > 0 &&
          <div id="logginStatus">
            <p>Inloggad som: {loggedInUsername}</p>
            <a href="/" onClick={logoutClick}>Logga Ut</a>
          </div>
        }
      </div>)
  }

  return (
    <div className="App">
      <div id="content">
        <div className="half">
          <h1>Förkorta en länk</h1>
          <p>Mata in länken i fältet nedan och tryck på knappen</p>
          <form onSubmit={shortenURL} autoComplete="off" id="shortenForm">
            <input type="text" value={longURL}
              onChange={e => setLongURL(e.target.value)}
              placeholder="Lång URL..."
              autoComplete="off"/>

            <button type="submit" >Förkorta</button>
          </form>

          { serverError.length > 0 && 
            <p className="serverError">{serverError}</p>}
          { history.length > 0 && serverError.length === 0 &&
          <div id="result">
            Din korta länk: <a target="_blank" href={server + history[history.length - 1].short}>localhost:8080/{history[history.length - 1].short}</a>
          </div>}
        </div>
        <div className="half right-half">
          { 
            <LoginSection />
          }
        </div>
      </div>

      {loggedInUsername.length > 0 && <Statistics history={history} setHistory={setHistory}/>}

    </div>
  );
}

export default App;
