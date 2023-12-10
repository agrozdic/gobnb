import { Component } from '@angular/core';
import { Router } from '@angular/router';
import { AuthService, UserService } from 'src/app/services';

@Component({
  selector: 'app-profile',
  templateUrl: './profile.component.html',
  styleUrls: ['./profile.component.css']
})
export class ProfileComponent {
  errorMessage: string | null = null;

  constructor(private userService: UserService, private router: Router, private authService: AuthService) {
    
  }

deleteProfile() {
    this.userService.deleteProfile().subscribe(
      () => {
        console.log('Profile deleted successfully')
        console.log("here")
        this.errorMessage = null;
        this.authService.logout();
        this.router.navigate(['/delete-account']);
      },
      error => {
        console.error('Failed to delete profile:', error);
          console.log('Error object:', error.error); // Log the entire error object


        if (error.status === 400 && error.error.message) {
          this.errorMessage = error.error.message;
        } else {
          this.errorMessage = "Failed to delete profile. Please try again.";
        }
      }
    );
  }
}
