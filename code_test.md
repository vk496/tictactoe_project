# Case study Backend Engineer   *(4 to 8 hours)*

You’ve been hired as a backend engineer at a game studio. As your first task, you are asked to design an API and implement the backend logic for a two-player tic-tac-toe game:

[https://www.mathsisfun.com/games/tic-tac-toe.html](https://www.mathsisfun.com/games/tic-tac-toe.html)

The game must support two real users playing concurrently in a turn-based manner.


## **Features**

Users should be able to:

* Start a new game and wait for an opponent  
* Search for pending (waiting) games  
* Join an existing pending game  
* Make moves when it is their turn  
* Win, lose, or draw a game  
* Retrieve their win/loss/draw statistics  

## **Technical Requirements**

* Use **Go** as the programming language  
* Prefer **gRPC** for API design (REST is also acceptable)  
* You may use any web framework  
* The **board size** and **winning length** must be configurable  
* The win/draw detection algorithm does **not** need to be optimized  
* Code should be **production-quality**  
* Design with **scalability** in mind (e.g. millions of users)  
* Keep all data **in memory** (no database required)  
* No authentication system is needed — users are identified by a provided user ID  
* No web interface or GUI is required

When a user starts a game, they should either:

* Create a new game and wait for an opponent, or  
* Join an existing game that is waiting for a second player  

## **Testing & Documentation**

* Include both **unit tests** and **acceptance tests**  
* Provide a **README** that explains:  
  * How to build and run the server  
  * Your design decisions and trade-offs


## **Please Do Not**

* Go beyond the defined scope  
* Implement a streaming API  
* Generate unit tests using AI  
* Spend time optimizing the win/loss/draw algorithm  

## **Submission Guidelines**

* You have **one week** to complete the assignment after receiving it  
  * If you need more time, please let us know  
* You may use AI tools, but:  
  * You must understand the code you submit  
  * Be prepared to explain your choices and discuss alternatives  
  * Your code should be human readable  
  * Avoid having comments as much as possible  
* Submit your work as a **.zip file** (via Google Drive, WeTransfer, or similar)  
* Send your submission using the link provided in **Ashby**